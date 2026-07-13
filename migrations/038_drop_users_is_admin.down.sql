-- Recreate the column structurally. The original per-user values are not
-- recoverable (they were split into is_system_admin / memberships.role and the
-- source flag was dropped), so it comes back default FALSE.
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE;
