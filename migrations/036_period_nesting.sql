-- +goose Up
-- +goose StatementBegin
ALTER TABLE periods ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE periods DROP COLUMN sort_order;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE periods ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE periods DROP COLUMN archived_at;

-- +goose StatementEnd
