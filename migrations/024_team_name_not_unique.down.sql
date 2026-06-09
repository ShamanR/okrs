-- Restore the unique constraint on team name. Fails if duplicate names exist.
ALTER TABLE teams ADD CONSTRAINT teams_name_key UNIQUE (name);
