-- Add external_id column and update source CHECK to include 'label'.
-- SQLite requires table recreation to modify CHECK constraints.

CREATE TABLE cert_monitors_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 443,
    source TEXT NOT NULL CHECK(source IN ('auto', 'standalone', 'label')),
    endpoint_id INTEGER REFERENCES endpoints(id),
    status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('valid', 'expiring', 'expired', 'error', 'unknown')),
    check_interval_seconds INTEGER NOT NULL DEFAULT 43200,
    warning_thresholds_json TEXT NOT NULL DEFAULT '[30,14,7,3,1]',
    last_alerted_threshold INTEGER,
    last_check_at INTEGER,
    next_check_at INTEGER,
    last_error TEXT,
    active INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    UNIQUE(hostname, port)
);

INSERT INTO cert_monitors_new (id, hostname, port, source, endpoint_id, status,
    check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
    last_check_at, next_check_at, last_error, active, created_at, external_id)
SELECT id, hostname, port, source, endpoint_id, status,
    check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
    last_check_at, next_check_at, last_error, active, created_at, ''
FROM cert_monitors;

DROP TABLE cert_monitors;
ALTER TABLE cert_monitors_new RENAME TO cert_monitors;

-- Recreate indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_cert_monitor_identity ON cert_monitors(hostname, port);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cert_monitor_active ON cert_monitors(active, status);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone', 'label') AND active = 1;
CREATE INDEX IF NOT EXISTS idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != '';
