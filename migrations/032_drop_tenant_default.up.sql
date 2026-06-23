-- All writes now pass tenant_id explicitly (Plan 2b). Drop the transitional DEFAULT 1
-- so a write that forgets tenant_id fails loudly instead of silently landing in tenant 1.
ALTER TABLE teams                  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE periods                ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goals                  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goal_shares            ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE team_period_statuses   ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE user_hierarchy_grants  ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE key_results            ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE goal_comments          ALTER COLUMN tenant_id DROP DEFAULT;
ALTER TABLE key_result_notes       ALTER COLUMN tenant_id DROP DEFAULT;
