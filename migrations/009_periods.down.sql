ALTER TABLE goals
  ADD COLUMN IF NOT EXISTS year INTEGER,
  ADD COLUMN IF NOT EXISTS quarter INTEGER;

UPDATE goals g
SET year = EXTRACT(YEAR FROM p.start_date)::int,
    quarter = ((EXTRACT(MONTH FROM p.start_date)::int - 1) / 3) + 1
FROM periods p
WHERE p.id = g.period_id;

ALTER TABLE goals
  ALTER COLUMN year SET NOT NULL,
  ALTER COLUMN quarter SET NOT NULL;

ALTER TABLE goals
  ADD CONSTRAINT goals_quarter_check CHECK (quarter BETWEEN 1 AND 4);

DROP INDEX IF EXISTS goals_team_period_idx;
DROP INDEX IF EXISTS goals_team_period_sort_idx;

CREATE INDEX IF NOT EXISTS goals_team_quarter_idx ON goals(team_id, year, quarter);
CREATE INDEX IF NOT EXISTS goals_team_quarter_sort_idx ON goals(team_id, year, quarter, sort_order);

CREATE TABLE IF NOT EXISTS team_quarter_statuses (
  team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  year INTEGER NOT NULL,
  quarter INTEGER NOT NULL CHECK (quarter BETWEEN 1 AND 4),
  status TEXT NOT NULL,
  PRIMARY KEY (team_id, year, quarter)
);

INSERT INTO team_quarter_statuses (team_id, year, quarter, status)
SELECT tps.team_id,
       EXTRACT(YEAR FROM p.start_date)::int,
       ((EXTRACT(MONTH FROM p.start_date)::int - 1) / 3) + 1,
       tps.status
FROM team_period_statuses tps
JOIN periods p ON p.id = tps.period_id
ON CONFLICT (team_id, year, quarter) DO UPDATE SET status = EXCLUDED.status;

DROP TABLE IF EXISTS team_period_statuses;

ALTER TABLE goals
  DROP CONSTRAINT IF EXISTS goals_period_fk;

ALTER TABLE goals
  DROP COLUMN IF EXISTS period_id;

DROP TABLE IF EXISTS periods;
