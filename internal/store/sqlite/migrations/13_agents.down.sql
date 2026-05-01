-- Remove partial indexes first (WHERE clause indexes may need explicit drop)
DROP INDEX IF EXISTS idx_cert_monitors_agent_id;
DROP INDEX IF EXISTS idx_resource_snapshots_agent_id;
DROP INDEX IF EXISTS idx_heartbeats_agent_id;
DROP INDEX IF EXISTS idx_endpoints_agent_id;
DROP INDEX IF EXISTS idx_containers_agent_id;

-- Remove multi-host attribution columns (SQLite >= 3.35 supports DROP COLUMN)
ALTER TABLE cert_monitors      DROP COLUMN agent_id;
ALTER TABLE resource_snapshots DROP COLUMN agent_id;
ALTER TABLE heartbeats         DROP COLUMN agent_id;
ALTER TABLE endpoints          DROP COLUMN agent_id;
ALTER TABLE containers         DROP COLUMN agent_id;

DROP INDEX IF EXISTS idx_enrollment_tokens_expires_at;
DROP INDEX IF EXISTS idx_agents_last_seen_at;
DROP INDEX IF EXISTS idx_agents_status;

DROP TABLE IF EXISTS enrollment_tokens;
DROP TABLE IF EXISTS agents;
