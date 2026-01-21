DROP INDEX IF EXISTS key_results_goal_sort_idx;
ALTER TABLE key_results
  DROP COLUMN IF EXISTS sort_order;
