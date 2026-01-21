CREATE TABLE IF NOT EXISTS team_quarter_statuses (
  team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  year INTEGER NOT NULL,
  quarter INTEGER NOT NULL CHECK (quarter BETWEEN 1 AND 4),
  status TEXT NOT NULL CHECK (status IN ('no_goals', 'forming', 'in_progress', 'validated', 'closed')),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (team_id, year, quarter)
);

CREATE INDEX IF NOT EXISTS team_quarter_statuses_team_idx ON team_quarter_statuses(team_id);
