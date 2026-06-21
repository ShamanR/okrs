ALTER TABLE periods DROP CONSTRAINT IF EXISTS periods_tenant_name_key;
ALTER TABLE periods ADD CONSTRAINT periods_name_key UNIQUE (name);
ALTER TABLE auth_sessions DROP COLUMN IF EXISTS active_tenant_id;
DROP TABLE IF EXISTS tenant_invitations;
