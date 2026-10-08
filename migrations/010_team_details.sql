-- +goose Up
-- +goose StatementBegin
ALTER TABLE teams
  ADD COLUMN lead TEXT NOT NULL DEFAULT '',
  ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teams
  DROP COLUMN IF EXISTS description,
  DROP COLUMN IF EXISTS lead;

-- +goose StatementEnd
