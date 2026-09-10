-- Local, agent-only spool. Holds events collected while the server was
-- unreachable, until an EventAck covers them.
--
-- AUTOINCREMENT is required, not decorative: a plain rowid is reused after the
-- highest row is deleted, and the drain cursor would then walk backwards.

CREATE TABLE spool_events (
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id    TEXT NOT NULL,
    observed_at BIGINT NOT NULL,
    payload     BLOB NOT NULL,
    size_bytes  INTEGER NOT NULL
);

CREATE INDEX spool_events_observed ON spool_events(observed_at);

CREATE TABLE spool_meta (
    key   TEXT PRIMARY KEY NOT NULL,
    value BIGINT NOT NULL DEFAULT 0
);

INSERT INTO spool_meta(key, value) VALUES ('dropped_total', 0), ('dropped_since_connect', 0);
