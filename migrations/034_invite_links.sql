-- +goose Up
-- +goose StatementBegin
-- Invite links: generic (no email), one-time or multi-use.
ALTER TABLE tenant_invitations ALTER COLUMN email DROP NOT NULL;
ALTER TABLE tenant_invitations ADD COLUMN max_uses INT;
ALTER TABLE tenant_invitations ADD COLUMN use_count INT NOT NULL DEFAULT 0;

-- Existing invitations were single-use; preserve that semantic.
UPDATE tenant_invitations SET max_uses = 1 WHERE max_uses IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE tenant_invitations SET email = '' WHERE email IS NULL;
ALTER TABLE tenant_invitations ALTER COLUMN email SET NOT NULL;
ALTER TABLE tenant_invitations DROP COLUMN use_count;
ALTER TABLE tenant_invitations DROP COLUMN max_uses;

-- +goose StatementEnd
