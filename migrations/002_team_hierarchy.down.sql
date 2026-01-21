DROP INDEX IF EXISTS teams_parent_id_idx;
ALTER TABLE teams
  DROP COLUMN IF EXISTS parent_id,
  DROP COLUMN IF EXISTS team_type;
