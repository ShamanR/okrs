-- +goose Up
-- +goose StatementBegin
ALTER TABLE teams
  ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS teams_deleted_at_idx ON teams(deleted_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS teams_deleted_at_idx;

ALTER TABLE teams
  DROP COLUMN IF EXISTS deleted_at;

-- +goose StatementEnd
