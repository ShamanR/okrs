-- +goose Up
-- +goose StatementBegin
-- Направление цели заменяет фокус. Шаг expand: новая колонка direction рядом с
-- focus_type. Старые экземпляры приложения во время rolling update продолжают читать и
-- писать focus_type, поэтому колонка остаётся до отдельной миграции contract.
ALTER TABLE goals
    ADD COLUMN direction TEXT NOT NULL DEFAULT 'PRODUCT'
        CONSTRAINT goals_direction_check CHECK (direction IN ('PRODUCT', 'TECH', 'PROCESS'));

-- Перевод фокуса в направление по утверждённой таблице. Неизвестное значение фокуса
-- (такого быть не должно) получает направление по умолчанию.
UPDATE goals SET direction = CASE focus_type
    WHEN 'PROFITABILITY'     THEN 'PRODUCT'
    WHEN 'STABILITY'         THEN 'TECH'
    WHEN 'TECH_INDEPENDENCE' THEN 'TECH'
    WHEN 'SPEED_EFFICIENCY'  THEN 'PROCESS'
    ELSE 'PRODUCT'
END;

-- Новый код focus_type не пишет. Пустая строка, а не NULL: старый экземпляр сканирует
-- колонку в string и на NULL упал бы, а пустой фокус он просто не показывает.
ALTER TABLE goals ALTER COLUMN focus_type SET DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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

-- +goose StatementEnd
