-- +goose Up
-- +goose StatementBegin
-- Пороги оценки прогресса переезжают из JSON-ключа health_checkin_config
-- (настройки удалённой сводки Health Check-in) в отдельные продуктовые ключи
-- раздела «Настройки». Переносятся только четыре порога, которыми пользуются
-- другие экраны; настройки самой сводки (comment_depth, resolved_comments_limit,
-- in_counter, cache_ttl_minutes) отбрасываются. Переносится только числовое
-- поле, которое действительно задано: отсутствующее значение продолжит
-- браться по умолчанию. Идемпотентно.
INSERT INTO tenant_settings (tenant_id, key, value_json)
SELECT s.tenant_id, m.new_key, s.value_json -> m.field
FROM tenant_settings s
CROSS JOIN (VALUES
    ('stale_days',       'progress_stale_days'),
    ('behind_margin',    'progress_behind_margin'),
    ('green_threshold',  'progress_green_threshold'),
    ('weight_tolerance', 'progress_weight_tolerance')
) AS m (field, new_key)
WHERE s.key = 'health_checkin_config'
  AND jsonb_typeof(s.value_json -> m.field) = 'number'
ON CONFLICT (tenant_id, key) DO NOTHING;

DELETE FROM tenant_settings WHERE key = 'health_checkin_config';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Собирает health_checkin_config обратно из отдельных ключей порогов.
-- Восстанавливаются только четыре порога; отброшенные при накате настройки
-- сводки не восстанавливаются — старый код подставит для них значения по
-- умолчанию.
INSERT INTO tenant_settings (tenant_id, key, value_json)
SELECT tenant_id, 'health_checkin_config', jsonb_object_agg(field, value_json)
FROM (
    SELECT tenant_id, value_json,
           CASE key
               WHEN 'progress_stale_days'       THEN 'stale_days'
               WHEN 'progress_behind_margin'    THEN 'behind_margin'
               WHEN 'progress_green_threshold'  THEN 'green_threshold'
               WHEN 'progress_weight_tolerance' THEN 'weight_tolerance'
           END AS field
    FROM tenant_settings
    WHERE key IN ('progress_stale_days', 'progress_behind_margin',
                  'progress_green_threshold', 'progress_weight_tolerance')
) t
GROUP BY tenant_id
ON CONFLICT (tenant_id, key) DO NOTHING;

DELETE FROM tenant_settings
WHERE key IN ('progress_stale_days', 'progress_behind_margin',
              'progress_green_threshold', 'progress_weight_tolerance');

-- +goose StatementEnd
