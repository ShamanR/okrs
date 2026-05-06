CREATE TABLE user_hierarchy_grants (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id             BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_user_id  BIGINT NOT NULL REFERENCES users(id),
    UNIQUE (user_id, team_id)
);

CREATE INDEX user_hierarchy_grants_user_id_idx ON user_hierarchy_grants (user_id);
