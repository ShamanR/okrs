DROP INDEX IF EXISTS goals_team_quarter_sort_idx;
ALTER TABLE goals DROP COLUMN IF EXISTS sort_order;
