ALTER TABLE key_results
  ADD COLUMN IF NOT EXISTS progress_updated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS key_results_progress_updated_at_idx
  ON key_results(progress_updated_at);
