-- Tenant-admin is tenant-scoped (memberships.role = admin); the legacy global users.is_admin
-- no longer gates tenant access. Backfill so users who were global admins keep admin rights in
-- the tenants they belong to.
UPDATE memberships m
SET role = 'admin'
FROM users u
WHERE u.id = m.user_id AND u.is_admin = true AND m.role <> 'admin';
