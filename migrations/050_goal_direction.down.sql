-- Возврат к фокусу. Фокус переписывается по направлению там, где направление — более
-- свежая правда, чем фокус:
--   * цели, созданные уже на направлениях: фокуса у них нет (focus_type = '');
--   * цели, которым после наката сменили направление: их фокус больше не переводится в
--     текущее направление по таблице из up-миграции.
-- Цели, чьё направление по-прежнему соответствует фокусу, не трогаются: так сохраняется
-- исходный фокус там, где таблица перевода склеивала значения (Stability и
-- Tech Independence оба перешли в TECH).
UPDATE goals SET focus_type = CASE direction
    WHEN 'TECH'    THEN 'STABILITY'
    WHEN 'PROCESS' THEN 'SPEED_EFFICIENCY'
    ELSE 'PROFITABILITY'
END
WHERE focus_type = ''
   OR direction <> CASE focus_type
        WHEN 'PROFITABILITY'     THEN 'PRODUCT'
        WHEN 'STABILITY'         THEN 'TECH'
        WHEN 'TECH_INDEPENDENCE' THEN 'TECH'
        WHEN 'SPEED_EFFICIENCY'  THEN 'PROCESS'
        ELSE 'PRODUCT'
      END;

ALTER TABLE goals ALTER COLUMN focus_type DROP DEFAULT;
ALTER TABLE goals DROP COLUMN direction;
