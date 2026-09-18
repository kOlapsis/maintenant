-- Migrations run before the one-time UUID conversion, which drops and recreates
-- `agents` from uuid_schema.sql: the stub only gives the ALTERs a target on a
-- database that has not been converted yet.
CREATE TABLE IF NOT EXISTS agents (id TEXT PRIMARY KEY NOT NULL);

ALTER TABLE agents ADD COLUMN os_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN os_version_id TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN os_pretty_name TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN os_source TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN os_unavailable_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN os_reported_at BIGINT;
