-- Возврат к фокусу. Цели, созданные уже на направлениях, фокуса не имеют
-- (focus_type = ''); им фокус проставляется по обратному соответствию направлению.
UPDATE goals SET focus_type = CASE direction
    WHEN 'TECH'    THEN 'STABILITY'
    WHEN 'PROCESS' THEN 'SPEED_EFFICIENCY'
    ELSE 'PROFITABILITY'
END
WHERE focus_type = '';

ALTER TABLE goals ALTER COLUMN focus_type DROP DEFAULT;
ALTER TABLE goals DROP COLUMN direction;
