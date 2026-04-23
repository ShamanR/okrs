-- Rename status 'validated' → 'ready' to match the new status model:
-- no_goals → forming → ready → in_progress → closed
UPDATE team_period_statuses SET status = 'ready' WHERE status = 'validated';
