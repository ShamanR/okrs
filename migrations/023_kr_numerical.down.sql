-- Recreate legacy tables.
CREATE TABLE IF NOT EXISTS kr_percent_meta (
  key_result_id INTEGER PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  start_value DOUBLE PRECISION NOT NULL,
  target_value DOUBLE PRECISION NOT NULL,
  current_value DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS kr_linear_meta (
  key_result_id INTEGER PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  start_value DOUBLE PRECISION NOT NULL,
  target_value DOUBLE PRECISION NOT NULL,
  current_value DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS kr_percent_checkpoints (
  id SERIAL PRIMARY KEY,
  key_result_id INTEGER NOT NULL REFERENCES key_results(id) ON DELETE CASCADE,
  metric_value DOUBLE PRECISION NOT NULL,
  kr_percent INTEGER NOT NULL CHECK (kr_percent BETWEEN 0 AND 100)
);

-- Revert NUMERICAL KRs that have checkpoints to PERCENT, the rest to LINEAR.
UPDATE key_results SET kind = 'PERCENT'
WHERE kind = 'NUMERICAL' AND checkpoints IS NOT NULL AND jsonb_array_length(checkpoints) > 0;
UPDATE key_results SET kind = 'LINEAR'
WHERE kind = 'NUMERICAL';

-- Restore meta rows.
INSERT INTO kr_percent_meta (key_result_id, start_value, target_value, current_value)
SELECT id, COALESCE(start_value, 0), COALESCE(target_value, 0), COALESCE(current_value, 0)
FROM key_results WHERE kind = 'PERCENT';

INSERT INTO kr_linear_meta (key_result_id, start_value, target_value, current_value)
SELECT id, COALESCE(start_value, 0), COALESCE(target_value, 0), COALESCE(current_value, 0)
FROM key_results WHERE kind = 'LINEAR';

-- Restore checkpoint rows.
INSERT INTO kr_percent_checkpoints (key_result_id, metric_value, kr_percent)
SELECT kr.id,
       (elem->>'value')::double precision,
       (elem->>'progress_percent')::int
FROM key_results kr
CROSS JOIN LATERAL jsonb_array_elements(kr.checkpoints) elem
WHERE kr.checkpoints IS NOT NULL;

ALTER TABLE key_results
  DROP COLUMN IF EXISTS unit,
  DROP COLUMN IF EXISTS start_value,
  DROP COLUMN IF EXISTS target_value,
  DROP COLUMN IF EXISTS current_value,
  DROP COLUMN IF EXISTS checkpoints,
  DROP COLUMN IF EXISTS zeroing_criteria;
