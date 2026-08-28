-- Уведомление адресовано конкретному получателю (user_id) и является одновременно
-- строкой in-app колокольчика. Ссылки на сущности намеренно НЕ внешние ключи:
-- уведомление переживает удаление цели, ровно как строка activity_events.
CREATE TABLE notifications (
    id             BIGSERIAL PRIMARY KEY,
    tenant_id      BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    type           TEXT   NOT NULL,
    kind           TEXT   NOT NULL,
    actor_user_id  BIGINT NOT NULL REFERENCES users(id),
    team_id        BIGINT,
    period_id      BIGINT,
    goal_id        BIGINT,
    kr_id          BIGINT,
    comment_id     BIGINT,
    entity_title   TEXT   NOT NULL DEFAULT '',
    payload_json   JSONB  NOT NULL DEFAULT '{}',
    coalesce_key   TEXT   NOT NULL,
    coalesce_count INT    NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at        TIMESTAMPTZ,
    CONSTRAINT notifications_type CHECK (
        type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress')),
    -- Ключ схлопывания: тип:сущность:актор:бакет. UNIQUE делает вставку
    -- идемпотентной между репликами — ON CONFLICT DO UPDATE вместо read-then-write.
    UNIQUE (tenant_id, user_id, coalesce_key)
);

-- Лента колокольчика: свежие сверху, в пределах одного получателя. id DESC замыкает
-- индекс под ORDER BY (created_at DESC, id DESC) — без него Postgres досортировывает
-- весь совпавший набор перед LIMIT (на PG11 нет incremental sort).
CREATE INDEX idx_notifications_feed ON notifications (tenant_id, user_id, created_at DESC, id DESC);
-- Бейдж: COUNT по частичному индексу, без чтения прочитанных.
CREATE INDEX idx_notifications_unread ON notifications (tenant_id, user_id) WHERE read_at IS NULL;
-- Ретенция чистит по updated_at, не created_at: коалесинг обновляет updated_at и
-- сбрасывает read_at, так что схлопывающееся уведомление не должно теряться из-под
-- непрочитанного бейджа только из-за старой даты первого возникновения.
CREATE INDEX idx_notifications_updated ON notifications (updated_at);

-- Настройки пользователя. Отсутствие строки = дефолт (включено, scope=own,
-- channels={in_app}), поэтому бэкфилл на всех пользователей не нужен.
-- tenant_id в ключе обязателен: человек состоит в нескольких пространствах.
CREATE TABLE notification_preferences (
    tenant_id BIGINT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id   BIGINT  NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    type      TEXT    NOT NULL,
    enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    scope     TEXT,
    channels  TEXT[]  NOT NULL DEFAULT '{in_app}',
    PRIMARY KEY (tenant_id, user_id, type),
    CONSTRAINT notification_preferences_type CHECK (
        type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress')),
    -- Скоуп неприменим к адресным типам: у my_comment_resolved он всегда NULL.
    CONSTRAINT notification_preferences_scope CHECK (
        scope IS NULL OR scope IN ('own','own_and_children','subtree'))
);
