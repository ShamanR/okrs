CREATE TABLE system_settings (
    key        TEXT PRIMARY KEY,
    value_json JSONB NOT NULL DEFAULT 'null'
);

INSERT INTO system_settings (key, value_json) VALUES
    ('new_user_policy', '"empty"'),
    ('default_hierarchy_node_id', 'null');
