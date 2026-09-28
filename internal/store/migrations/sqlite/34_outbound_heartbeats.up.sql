CREATE TABLE outbound_heartbeats (
    id               TEXT PRIMARY KEY NOT NULL,
    name             TEXT NOT NULL,
    url              TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    last_sent_at     BIGINT,
    last_status_code INTEGER,
    last_error       TEXT,
    created_at       BIGINT NOT NULL,
    updated_at       BIGINT NOT NULL
);
