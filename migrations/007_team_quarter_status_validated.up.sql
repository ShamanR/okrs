ALTER TABLE team_quarter_statuses
  DROP CONSTRAINT IF EXISTS team_quarter_statuses_status_check;

ALTER TABLE team_quarter_statuses
  ADD CONSTRAINT team_quarter_statuses_status_check
  CHECK (status IN ('no_goals', 'forming', 'in_progress', 'validated', 'closed'));
