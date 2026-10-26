-- +goose Up
-- +goose StatementBegin
ALTER TABLE key_results
  ADD COLUMN IF NOT EXISTS progress_updated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS key_results_progress_updated_at_idx
  ON key_results(progress_updated_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS key_results_progress_updated_at_idx;

ALTER TABLE key_results
  DROP COLUMN IF EXISTS progress_updated_at;

-- +goose StatementEnd
