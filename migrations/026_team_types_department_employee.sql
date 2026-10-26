-- +goose Up
-- +goose StatementBegin
ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_team_type_check;
ALTER TABLE teams
  ADD CONSTRAINT teams_team_type_check
  CHECK (team_type IN ('department', 'cluster', 'unit', 'group', 'team', 'squad', 'employee'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_team_type_check;
ALTER TABLE teams
  ADD CONSTRAINT teams_team_type_check
  CHECK (team_type IN ('cluster', 'unit', 'group', 'team', 'squad'));

-- +goose StatementEnd
