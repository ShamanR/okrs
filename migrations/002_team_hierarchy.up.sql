ALTER TABLE teams
  ADD COLUMN team_type TEXT NOT NULL DEFAULT 'team' CHECK (team_type IN ('cluster', 'unit', 'team')),
  ADD COLUMN parent_id INTEGER REFERENCES teams(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS teams_parent_id_idx ON teams(parent_id);
