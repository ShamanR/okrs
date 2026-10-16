-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
ALTER TABLE users ADD COLUMN udid UUID NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX users_udid_idx ON users (udid);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS users_udid_idx;
ALTER TABLE users DROP COLUMN IF EXISTS udid;

-- +goose StatementEnd
