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
