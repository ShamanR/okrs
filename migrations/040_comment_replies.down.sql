DROP INDEX IF EXISTS idx_goal_comments_parent;
ALTER TABLE goal_comments DROP COLUMN IF EXISTS parent_id;
