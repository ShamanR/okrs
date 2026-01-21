ALTER TABLE goals
  ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY team_id, year, quarter ORDER BY created_at, id) AS rn
  FROM goals
)
UPDATE goals
SET sort_order = ranked.rn
FROM ranked
WHERE goals.id = ranked.id;

CREATE INDEX IF NOT EXISTS goals_team_quarter_sort_idx ON goals(team_id, year, quarter, sort_order);
