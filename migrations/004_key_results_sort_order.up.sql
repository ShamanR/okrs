ALTER TABLE key_results
  ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE key_results
SET sort_order = id
WHERE sort_order = 0;

CREATE INDEX IF NOT EXISTS key_results_goal_sort_idx ON key_results(goal_id, sort_order);
