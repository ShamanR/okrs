package settings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsRepository handles system_settings persistence.
type SettingsRepository struct {
	db *pgxpool.Pool
}

func NewSettingsRepository(db *pgxpool.Pool) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	row := r.db.QueryRow(ctx, `SELECT value_json FROM system_settings WHERE key = $1`, key)
	var raw json.RawMessage
	err := row.Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return raw, err
}

func (r *SettingsRepository) SetSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO system_settings (key, value_json) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value_json = EXCLUDED.value_json`,
		key, raw)
	return err
}
