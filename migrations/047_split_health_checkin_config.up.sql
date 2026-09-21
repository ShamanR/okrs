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
