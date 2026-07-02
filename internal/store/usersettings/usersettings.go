package usersettings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserSettingsRepository persists per-user key/value preferences. Not on the hot
// path (loaded only on /settings), so it has no cache.
type UserSettingsRepository struct {
	db *pgxpool.Pool
}

func NewUserSettingsRepository(db *pgxpool.Pool) *UserSettingsRepository {
	return &UserSettingsRepository{db: db}
}

func (r *UserSettingsRepository) GetAll(ctx context.Context, userID int64) (map[string]json.RawMessage, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value_json FROM user_settings WHERE user_id = $1`, userID)
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

func (r *UserSettingsRepository) Get(ctx context.Context, userID int64, key string) (json.RawMessage, error) {
	row := r.db.QueryRow(ctx, `SELECT value_json FROM user_settings WHERE user_id = $1 AND key = $2`, userID, key)
	var raw json.RawMessage
	err := row.Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (r *UserSettingsRepository) Set(ctx context.Context, userID int64, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO user_settings (user_id, key, value_json) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO UPDATE SET value_json = EXCLUDED.value_json`,
		userID, key, raw)
	return err
}
