ALTER TABLE teams
  ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS teams_deleted_at_idx ON teams(deleted_at);
