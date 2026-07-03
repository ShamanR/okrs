ALTER TABLE periods ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE periods DROP COLUMN sort_order;
