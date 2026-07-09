ALTER TABLE goal_comments ADD COLUMN resolved_at TIMESTAMPTZ;
ALTER TABLE goal_comments ADD COLUMN resolved_by_user_id BIGINT REFERENCES users(id);
