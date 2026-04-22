CREATE TABLE users (
    id                  BIGSERIAL PRIMARY KEY,
    provider_subject_key TEXT NOT NULL UNIQUE,
    provider            TEXT NOT NULL,
    subject             TEXT NOT NULL,
    display_name        TEXT NOT NULL,
    avatar_url          TEXT NOT NULL DEFAULT '',
    email               TEXT,
    attributes_json     JSONB NOT NULL DEFAULT '{}',
    is_admin            BOOLEAN NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX users_provider_subject_idx ON users (provider, subject);

-- System users: anonymous-local (no-auth mode) and migration (backfill)
INSERT INTO users (id, provider_subject_key, provider, subject, display_name, avatar_url, is_admin)
VALUES
    (1, 'system:anonymous-local', 'system', 'anonymous-local', 'Anonymous', '', FALSE),
    (2, 'system:migration',       'system', 'migration',       'Migration',  '', FALSE);

SELECT setval('users_id_seq', 100);
