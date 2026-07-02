UPDATE tenant_invitations SET email = '' WHERE email IS NULL;
ALTER TABLE tenant_invitations ALTER COLUMN email SET NOT NULL;
ALTER TABLE tenant_invitations DROP COLUMN use_count;
ALTER TABLE tenant_invitations DROP COLUMN max_uses;
