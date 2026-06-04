DROP INDEX IF EXISTS teams_lead_udid_idx;
ALTER TABLE goals DROP COLUMN IF EXISTS owner_udids;
ALTER TABLE teams DROP COLUMN IF EXISTS lead_udid;
