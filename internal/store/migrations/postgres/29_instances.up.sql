-- Instances visibility (FR-012): each running instance registers itself and
-- beats periodically. The table informs, it never arbitrates: no lock, no
-- lease, no election. On SQLite it never holds more than one row.
CREATE TABLE instances (
    id            TEXT PRIMARY KEY NOT NULL,                -- ephemeral, minted at startup
    hostname      TEXT NOT NULL,
    version       TEXT NOT NULL,
    started_at    BIGINT NOT NULL,
    last_seen_at  BIGINT NOT NULL
);
CREATE INDEX idx_instances_last_seen_at ON instances(last_seen_at);
