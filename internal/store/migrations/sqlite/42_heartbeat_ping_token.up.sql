ALTER TABLE heartbeats ADD COLUMN ping_token TEXT NOT NULL DEFAULT '';
UPDATE heartbeats SET ping_token = id;
CREATE UNIQUE INDEX uq_heartbeats_ping_token ON heartbeats(ping_token);
