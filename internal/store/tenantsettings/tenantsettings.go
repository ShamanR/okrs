package tenantsettings

import (
	"context"
	"encoding/json"

	"okrs/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantSettingsRepository persists per-tenant key/value settings (product keys
// + entitlement.* keys). It is policy-free; write-authority is enforced in the
// service layer.
type TenantSettingsRepository struct {
	db *pgxpool.Pool
}

func NewTenantSettingsRepository(db *pgxpool.Pool) *TenantSettingsRepository {
	return &TenantSettingsRepository{db: db}
}

// GetAll loads the whole tenant snapshot in one query.
func (r *TenantSettingsRepository) GetAll(ctx context.Context, scope domain.TenantScope) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value_json FROM tenant_settings WHERE tenant_id = $1`, scope.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *TenantSettingsRepository) Get(ctx context.Context, scope domain.TenantScope, key string) (json.RawMessage, error) {
	row := r.db.QueryRow(ctx, `SELECT value_json FROM tenant_settings WHERE tenant_id = $1 AND key = $2`, scope.TenantID, key)
	var raw json.RawMessage
	err := row.Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (r *TenantSettingsRepository) Set(ctx context.Context, scope domain.TenantScope, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO tenant_settings (tenant_id, key, value_json) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`,
		scope.TenantID, key, raw)
	return err
}

func (r *TenantSettingsRepository) Delete(ctx context.Context, scope domain.TenantScope, key string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM tenant_settings WHERE tenant_id = $1 AND key = $2`, scope.TenantID, key)
	return err
}
