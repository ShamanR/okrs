ALTER TABLE goal_comments
  ADD COLUMN parent_id BIGINT NULL REFERENCES goal_comments(id) ON DELETE CASCADE;
CREATE INDEX idx_goal_comments_parent ON goal_comments(goal_id, parent_id, created_at);
