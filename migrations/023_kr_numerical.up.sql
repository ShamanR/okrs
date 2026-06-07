-- Add NUMERICAL columns directly on key_results.
ALTER TABLE key_results
  ADD COLUMN IF NOT EXISTS unit TEXT,
  ADD COLUMN IF NOT EXISTS start_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS target_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS current_value DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS checkpoints JSONB,
  ADD COLUMN IF NOT EXISTS zeroing_criteria TEXT NOT NULL DEFAULT '';

-- Backfill scalar values from the legacy LINEAR meta table.
UPDATE key_results kr
SET start_value = m.start_value,
    target_value = m.target_value,
    current_value = m.current_value,
    unit = '%'
FROM kr_linear_meta m
WHERE m.key_result_id = kr.id;

-- Backfill scalar values from the legacy PERCENT meta table.
UPDATE key_results kr
SET start_value = m.start_value,
    target_value = m.target_value,
    current_value = m.current_value,
    unit = '%'
FROM kr_percent_meta m
WHERE m.key_result_id = kr.id;

-- Backfill PERCENT checkpoints into the JSONB column (value/progress_percent).
UPDATE key_results kr
SET checkpoints = c.points
FROM (
  SELECT key_result_id,
         jsonb_agg(jsonb_build_object('value', metric_value, 'progress_percent', kr_percent)
                   ORDER BY metric_value) AS points
  FROM kr_percent_checkpoints
  GROUP BY key_result_id
) c
WHERE c.key_result_id = kr.id;

-- Guard partially-created legacy KRs that have no meta row. The legacy meta
-- tables are optional (no NOT NULL FK from key_results), and CreateKeyResultWithMeta
-- writes the KR before its meta, so a row can exist with no meta. For those the
-- backfill above leaves start/target/current NULL, which load as 0/0/0.
-- NumericalProgress treats start == target with current >= target as 100%, so
-- without sane defaults these malformed rows would jump from 0% to 100% and
-- inflate goal progress. Default them to an empty 0..100 range so they keep
-- reading as 0%.
UPDATE key_results
SET start_value = 0,
    target_value = 100,
    current_value = 0,
    unit = COALESCE(unit, '%')
WHERE kind IN ('LINEAR', 'PERCENT') AND target_value IS NULL;

-- Flip legacy kinds to NUMERICAL (preserves all other KR data).
UPDATE key_results SET kind = 'NUMERICAL' WHERE kind IN ('LINEAR', 'PERCENT');

-- Drop legacy tables.
DROP TABLE IF EXISTS kr_percent_checkpoints;
DROP TABLE IF EXISTS kr_percent_meta;
DROP TABLE IF EXISTS kr_linear_meta;
