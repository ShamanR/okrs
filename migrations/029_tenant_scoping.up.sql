-- Scope every tenant-owned table by tenant_id. DEFAULT 1 keeps existing single-tenant
-- writes working and backfills existing rows; the default is transitional (removed once
-- all writes are tenant-aware). tenant_id is denormalized onto child tables (key_results,
-- goal_comments, key_result_notes) for defense-in-depth: every query carries tenant_id.
ALTER TABLE teams                  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE periods                ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goals                  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goal_shares            ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE team_period_statuses   ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE user_hierarchy_grants  ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE key_results            ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE goal_comments          ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);
ALTER TABLE key_result_notes       ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 1 REFERENCES tenants(id);

CREATE INDEX idx_teams_tenant                 ON teams(tenant_id);
CREATE INDEX idx_periods_tenant               ON periods(tenant_id);
CREATE INDEX idx_goals_tenant                 ON goals(tenant_id);
CREATE INDEX idx_goal_shares_tenant           ON goal_shares(tenant_id);
CREATE INDEX idx_team_period_statuses_tenant  ON team_period_statuses(tenant_id);
CREATE INDEX idx_user_hierarchy_grants_tenant ON user_hierarchy_grants(tenant_id);
CREATE INDEX idx_key_results_tenant           ON key_results(tenant_id);
CREATE INDEX idx_goal_comments_tenant         ON goal_comments(tenant_id);
CREATE INDEX idx_key_result_notes_tenant      ON key_result_notes(tenant_id);
