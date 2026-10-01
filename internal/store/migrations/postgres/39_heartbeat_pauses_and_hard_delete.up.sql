CREATE TABLE heartbeat_pauses (
    id           TEXT PRIMARY KEY NOT NULL,
    heartbeat_id TEXT NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    paused_at    BIGINT NOT NULL,
    resumed_at   BIGINT
);
CREATE INDEX idx_hb_pause_heartbeat ON heartbeat_pauses(heartbeat_id, paused_at);

DELETE FROM heartbeats WHERE active = 0;

INSERT INTO heartbeat_pauses (id, heartbeat_id, paused_at)
SELECT id, id, updated_at FROM heartbeats WHERE status = 'paused';

DROP INDEX IF EXISTS idx_heartbeat_status_deadline;
DROP INDEX IF EXISTS idx_heartbeat_active;
ALTER TABLE heartbeats DROP COLUMN active;
CREATE INDEX idx_heartbeat_status_deadline ON heartbeats(status, next_deadline_at);
