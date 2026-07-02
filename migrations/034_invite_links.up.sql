-- Invite links: generic (no email), one-time or multi-use.
ALTER TABLE tenant_invitations ALTER COLUMN email DROP NOT NULL;
ALTER TABLE tenant_invitations ADD COLUMN max_uses INT;
ALTER TABLE tenant_invitations ADD COLUMN use_count INT NOT NULL DEFAULT 0;

-- Existing invitations were single-use; preserve that semantic.
UPDATE tenant_invitations SET max_uses = 1 WHERE max_uses IS NULL;
