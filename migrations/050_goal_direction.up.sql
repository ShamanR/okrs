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
