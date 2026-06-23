-- Move product keys out of the global system_settings into per-tenant tenant_settings
-- (default tenant #1). system_settings keeps only instance-global keys. Idempotent.
INSERT INTO tenant_settings (tenant_id, key, value_json)
SELECT 1, key, value_json FROM system_settings
WHERE key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
)
ON CONFLICT (tenant_id, key) DO NOTHING;

DELETE FROM system_settings
WHERE key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
);
