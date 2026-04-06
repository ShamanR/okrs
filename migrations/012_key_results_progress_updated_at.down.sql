DROP INDEX IF EXISTS key_results_progress_updated_at_idx;

ALTER TABLE key_results
  DROP COLUMN IF EXISTS progress_updated_at;
