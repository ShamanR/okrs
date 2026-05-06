CREATE EXTENSION IF NOT EXISTS pgcrypto;
ALTER TABLE users ADD COLUMN udid UUID NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX users_udid_idx ON users (udid);
