-- +goose Up
-- +goose StatementBegin
-- Активное пространство перестало быть состоянием сессии: контекст вкладки живёт
-- на клиенте (sessionStorage + X-Tenant-ID), сессия описывает только идентичность.
ALTER TABLE auth_sessions DROP COLUMN IF EXISTS active_tenant_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE auth_sessions ADD COLUMN IF NOT EXISTS active_tenant_id BIGINT REFERENCES tenants(id);
-- +goose StatementEnd
