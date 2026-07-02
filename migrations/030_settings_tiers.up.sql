-- Three settings tiers: system_settings (global, system-admin) stays as-is for global keys;
-- tenant_settings (per-tenant, tenant-admin, plus entitlement.* keys) is new;
-- user_settings (per-user) is new.
--
-- NOTE: product keys currently in system_settings are NOT moved here. The running code
-- still reads them from system_settings; moving them without repointing those reads would
-- silently revert settings to defaults. The move + read repointing happen together in a
-- later plan (settings tier / Entitlements), keeping this plan behavior-preserving.
CREATE TABLE tenant_settings (
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, key)
);

CREATE TABLE user_settings (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value_json JSONB NOT NULL,
    PRIMARY KEY (user_id, key)
);
