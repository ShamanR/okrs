-- +goose Up
-- +goose StatementBegin
-- Drop the legacy global users.is_admin flag. Admin is now fully modeled by
-- users.is_system_admin (instance superadmin) and memberships.role = 'admin'
-- (tenant admin). Superadmins were backfilled to is_system_admin (028) and
-- tenant admins to memberships.role (035), so no data is lost by dropping it.
ALTER TABLE users DROP COLUMN is_admin;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Recreate the column structurally. The original per-user values are not
-- recoverable (they were split into is_system_admin / memberships.role and the
-- source flag was dropped), so it comes back default FALSE.
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose StatementEnd
