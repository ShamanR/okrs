CREATE TABLE IF NOT EXISTS goal_links (
    tenant_id      BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    child_goal_id  BIGINT NOT NULL REFERENCES goals(id)   ON DELETE CASCADE,
    parent_goal_id BIGINT NOT NULL REFERENCES goals(id)   ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, child_goal_id, parent_goal_id),
    CONSTRAINT goal_links_no_self CHECK (child_goal_id <> parent_goal_id)
);

-- Обход "детей" (по parent) и "родителей" (по child) — оба tenant-scoped.
CREATE INDEX IF NOT EXISTS idx_goal_links_parent ON goal_links (tenant_id, parent_goal_id);
CREATE INDEX IF NOT EXISTS idx_goal_links_child  ON goal_links (tenant_id, child_goal_id);
