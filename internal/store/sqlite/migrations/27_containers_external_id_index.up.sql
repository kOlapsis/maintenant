-- The update intelligence tables (image_updates, container_cves, version_pins,
-- risk_score_history) reference containers by external_id, which so far only
-- existed as the second column of the (agent_id, external_id) unique index —
-- unusable for a lookup by external_id alone.
CREATE INDEX IF NOT EXISTS idx_containers_external_id ON containers(external_id);
