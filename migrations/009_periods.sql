-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS periods (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE goals
  ADD COLUMN IF NOT EXISTS period_id INTEGER;

WITH distinct_periods AS (
  SELECT DISTINCT year, quarter FROM goals
  UNION
  SELECT DISTINCT year, quarter FROM team_quarter_statuses
)
INSERT INTO periods (name, start_date, end_date, sort_order)
SELECT
  CONCAT(dp.year, ' Q', dp.quarter),
  make_date(dp.year, (dp.quarter - 1) * 3 + 1, 1),
  (make_date(dp.year, (dp.quarter - 1) * 3 + 1, 1) + interval '3 months - 1 day')::date,
  ROW_NUMBER() OVER (ORDER BY dp.year, dp.quarter)
FROM distinct_periods dp
ON CONFLICT (name) DO NOTHING;

UPDATE goals g
SET period_id = p.id
FROM periods p
WHERE p.name = CONCAT(g.year, ' Q', g.quarter);

ALTER TABLE goals
  ALTER COLUMN period_id SET NOT NULL;

ALTER TABLE goals
  ADD CONSTRAINT goals_period_fk FOREIGN KEY (period_id) REFERENCES periods(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS goals_team_quarter_idx;
DROP INDEX IF EXISTS goals_team_quarter_sort_idx;

CREATE INDEX IF NOT EXISTS goals_team_period_idx ON goals(team_id, period_id);
CREATE INDEX IF NOT EXISTS goals_team_period_sort_idx ON goals(team_id, period_id, sort_order);

WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY team_id, period_id ORDER BY sort_order, created_at, id) AS rn
  FROM goals
)
UPDATE goals
SET sort_order = ranked.rn
FROM ranked
WHERE goals.id = ranked.id;

CREATE TABLE IF NOT EXISTS team_period_statuses (
  team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  period_id INTEGER NOT NULL REFERENCES periods(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  PRIMARY KEY (team_id, period_id)
);

INSERT INTO team_period_statuses (team_id, period_id, status)
SELECT tqs.team_id, p.id, tqs.status
FROM team_quarter_statuses tqs
JOIN periods p ON p.name = CONCAT(tqs.year, ' Q', tqs.quarter)
ON CONFLICT (team_id, period_id) DO UPDATE SET status = EXCLUDED.status;

DROP TABLE IF EXISTS team_quarter_statuses;

ALTER TABLE goals
  DROP COLUMN IF EXISTS year,
  DROP COLUMN IF EXISTS quarter;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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

-- +goose StatementEnd
