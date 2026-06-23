-- Reverse 033: copy product keys back from tenant_settings (tenant #1) into system_settings.
INSERT INTO system_settings (key, value_json)
SELECT key, value_json FROM tenant_settings
WHERE tenant_id = 1 AND key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
)
ON CONFLICT (key) DO NOTHING;

DELETE FROM tenant_settings
WHERE tenant_id = 1 AND key IN (
    'new_user_policy', 'default_hierarchy_node_id', 'documentation_url',
    'feedback_url', 'feedback_popup_enabled', 'feedback_menu_link_enabled',
    'feedback_frequency_days', 'health_checkin_config'
);
