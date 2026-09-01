// Package notificationchannels persists per-tenant channel configuration and the
// account links a channel needs to address a user.
//
// The secret is stored already encrypted; this package never sees plaintext and
// never decrypts — that belongs to service/notificationchannel, which owns the key.
package notificationchannels

import (
	"context"
	"encoding/json"
	"time"

	"okrs/internal/core/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Config is one channel's configuration inside a tenant.
type Config struct {
	Channel         string
	Enabled         bool
	Values          map[string]any
	SecretEnc       []byte
	SecretHint      string
	UpdatedAt       time.Time
	UpdatedByUserID *int64
}

// Identity links a user to their account in an external channel.
type Identity struct {
	UserID           int64
	Channel          string
	ExternalID       string
	ExternalUsername string
	LinkedAt         time.Time
}

const configCols = `channel, enabled, config_json, secret_enc, secret_hint, updated_at, updated_by_user_id`

func scanConfig(row pgx.Row) (Config, error) {
	var c Config
	var raw []byte
	err := row.Scan(&c.Channel, &c.Enabled, &raw, &c.SecretEnc, &c.SecretHint, &c.UpdatedAt, &c.UpdatedByUserID)
	if err != nil {
		return Config{}, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c.Values)
	}
	if c.Values == nil {
		c.Values = map[string]any{}
	}
	return c, nil
}

// List returns every channel the tenant has ever configured, enabled or not.
func (r *Repository) List(ctx context.Context, scope domain.TenantScope) ([]Config, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+configCols+` FROM notification_channels WHERE tenant_id = $1 ORDER BY channel`,
		scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Config
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one channel's configuration. A missing row is not an error: a tenant
// that never configured the channel is the normal case.
func (r *Repository) Get(ctx context.Context, scope domain.TenantScope, channel string) (Config, bool, error) {
	c, err := scanConfig(r.db.QueryRow(ctx,
		`SELECT `+configCols+` FROM notification_channels WHERE tenant_id = $1 AND channel = $2`,
		scope.TenantID, channel))
	if err == pgx.ErrNoRows {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

// Upsert writes the configuration, recording who changed it.
func (r *Repository) Upsert(ctx context.Context, scope domain.TenantScope, c Config, byUserID int64) error {
	values := c.Values
	if values == nil {
		values = map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO notification_channels
			(tenant_id, channel, enabled, config_json, secret_enc, secret_hint, updated_at, updated_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6, now(), $7)
		ON CONFLICT (tenant_id, channel) DO UPDATE
		   SET enabled            = EXCLUDED.enabled,
		       config_json        = EXCLUDED.config_json,
		       secret_enc         = EXCLUDED.secret_enc,
		       secret_hint        = EXCLUDED.secret_hint,
		       updated_at         = now(),
		       updated_by_user_id = EXCLUDED.updated_by_user_id`,
		scope.TenantID, c.Channel, c.Enabled, raw, c.SecretEnc, c.SecretHint, byUserID)
	return err
}

// GetIdentity returns a user's account link for one channel.
func (r *Repository) GetIdentity(ctx context.Context, scope domain.TenantScope, userID int64, channel string) (Identity, bool, error) {
	var id Identity
	var username *string
	err := r.db.QueryRow(ctx, `
		SELECT user_id, channel, external_id, external_username, linked_at
		  FROM notification_identities
		 WHERE tenant_id = $1 AND user_id = $2 AND channel = $3`,
		scope.TenantID, userID, channel,
	).Scan(&id.UserID, &id.Channel, &id.ExternalID, &username, &id.LinkedAt)
	if err == pgx.ErrNoRows {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	if username != nil {
		id.ExternalUsername = *username
	}
	return id, true, nil
}

// UpsertIdentity stores or refreshes a link. The unique index on
// (tenant_id, channel, external_id) rejects the same external account being
// claimed by a second user — that would misdeliver one person's notifications.
func (r *Repository) UpsertIdentity(ctx context.Context, scope domain.TenantScope, id Identity) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_identities
			(tenant_id, user_id, channel, external_id, external_username)
		VALUES ($1,$2,$3,$4,NULLIF($5,''))
		ON CONFLICT (tenant_id, user_id, channel) DO UPDATE
		   SET external_id       = EXCLUDED.external_id,
		       external_username = EXCLUDED.external_username,
		       linked_at         = now()`,
		scope.TenantID, id.UserID, id.Channel, id.ExternalID, id.ExternalUsername)
	return err
}
