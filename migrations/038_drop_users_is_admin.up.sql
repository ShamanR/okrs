-- Drop the legacy global users.is_admin flag. Admin is now fully modeled by
-- users.is_system_admin (instance superadmin) and memberships.role = 'admin'
-- (tenant admin). Superadmins were backfilled to is_system_admin (028) and
-- tenant admins to memberships.role (035), so no data is lost by dropping it.
ALTER TABLE users DROP COLUMN is_admin;
