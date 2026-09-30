DROP INDEX IF EXISTS idx_heartbeat_status_deadline;
ALTER TABLE heartbeats ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
CREATE INDEX idx_heartbeat_status_deadline ON heartbeats(status, next_deadline_at) WHERE active=1;
CREATE INDEX idx_heartbeat_active ON heartbeats(active);

DROP TABLE IF EXISTS heartbeat_pauses;
