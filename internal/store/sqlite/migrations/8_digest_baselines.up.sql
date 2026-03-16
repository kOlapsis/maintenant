CREATE TABLE IF NOT EXISTS digest_baselines (
    container_id TEXT PRIMARY KEY,
    image        TEXT NOT NULL,
    tag          TEXT NOT NULL,
    remote_digest TEXT NOT NULL,
    checked_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
