-- +goose Up
-- +goose StatementBegin
ALTER TABLE goal_comments
  ADD COLUMN parent_id BIGINT NULL REFERENCES goal_comments(id) ON DELETE CASCADE;
CREATE INDEX idx_goal_comments_parent ON goal_comments(goal_id, parent_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_goal_comments_parent;
ALTER TABLE goal_comments DROP COLUMN IF EXISTS parent_id;

-- +goose StatementEnd
