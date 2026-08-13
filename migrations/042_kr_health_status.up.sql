ALTER TABLE key_results
    ADD COLUMN IF NOT EXISTS health_status TEXT NOT NULL DEFAULT 'not_started';

-- Backfill: KRs already at 100% progress become 'done' (consistent with the 100%->done rule).
-- Progress is derived, so the predicate mirrors okr.*Progress at the 100% boundary per kind.

-- BOOLEAN: done when its boolean meta is_done = true.
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'BOOLEAN'
  AND EXISTS (SELECT 1 FROM kr_boolean_meta b WHERE b.key_result_id = kr.id AND b.is_done);

-- PROJECT: done when the sum of completed stage weights >= 100 (ProjectProgress clamps to 100).
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'PROJECT'
  AND COALESCE((SELECT SUM(s.weight) FROM kr_project_stages s
                WHERE s.key_result_id = kr.id AND s.is_done), 0) >= 100;

-- NUMERICAL: target is always the 100% point (with or without checkpoints); done when current
-- reached target in the goal direction. Increasing: target >= start & current >= target.
-- Decreasing: target < start & current <= target.
UPDATE key_results kr
SET health_status = 'done'
WHERE kr.kind = 'NUMERICAL'
  AND kr.start_value IS NOT NULL AND kr.target_value IS NOT NULL AND kr.current_value IS NOT NULL
  AND (
        (kr.target_value >= kr.start_value AND kr.current_value >= kr.target_value)
     OR (kr.target_value <  kr.start_value AND kr.current_value <= kr.target_value)
  );
