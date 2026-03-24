DROP INDEX IF EXISTS teams_deleted_at_idx;

ALTER TABLE teams
  DROP COLUMN IF EXISTS deleted_at;
