-- +goose Up
-- +goose StatementBegin
ALTER TABLE team_period_statuses ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE team_period_statuses DROP COLUMN IF EXISTS updated_at;

-- +goose StatementEnd
