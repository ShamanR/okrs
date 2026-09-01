-- Конфигурация канала в конкретном тенанте. Секрет лежит зашифрованным
-- (AES-256-GCM, nonce внутри значения); наружу в API уходит только secret_hint.
-- Имя канала — свободный текст без CHECK: набор каналов задаётся сборкой
-- (app.Config.NotificationChannels), и канал из внешнего репозитория обязан
-- работать без миграции.
CREATE TABLE notification_channels (
    tenant_id          BIGINT  NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel            TEXT    NOT NULL,
    enabled            BOOLEAN NOT NULL DEFAULT FALSE,
    config_json        JSONB   NOT NULL DEFAULT '{}',
    secret_enc         BYTEA,
    secret_hint        TEXT    NOT NULL DEFAULT '',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (tenant_id, channel)
);

-- Привязка аккаунта пользователя к внешнему каналу. Для Mattermost заполняется
-- автоматически при первой отправке (кэш резолва по email); для каналов с
-- Linker — по одноразовому токену.
CREATE TABLE notification_identities (
    tenant_id         BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    channel           TEXT   NOT NULL,
    external_id       TEXT   NOT NULL,
    external_username TEXT,
    linked_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, channel),
    -- Один внешний аккаунт не может принадлежать двум пользователям тенанта:
    -- иначе уведомления одного человека уедут другому.
    UNIQUE (tenant_id, channel, external_id)
);
