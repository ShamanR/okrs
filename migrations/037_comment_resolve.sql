-- +goose Up
-- +goose StatementBegin
ALTER TABLE goal_comments ADD COLUMN resolved_at TIMESTAMPTZ;
ALTER TABLE goal_comments ADD COLUMN resolved_by_user_id BIGINT REFERENCES users(id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE goal_comments DROP COLUMN resolved_by_user_id;
ALTER TABLE goal_comments DROP COLUMN resolved_at;

-- +goose StatementEnd
