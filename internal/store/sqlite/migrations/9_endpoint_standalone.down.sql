-- SQLite does not support DROP COLUMN before 3.35.0; recreate table
CREATE TABLE endpoints_backup AS SELECT
    id, container_name, label_key, external_id, endpoint_type, target,
    status, alert_state, consecutive_failures, consecutive_successes,
    last_check_at, last_response_time_ms, last_http_status, last_error,
    config_json, active, first_seen_at, last_seen_at,
    orchestration_group, orchestration_unit
FROM endpoints;

DROP TABLE endpoints;

CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_name TEXT NOT NULL,
    label_key TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    endpoint_type TEXT NOT NULL CHECK(endpoint_type IN ('http', 'tcp')),
    target TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('up', 'down', 'unknown')),
    alert_state TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal', 'alerting')),
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_check_at INTEGER,
    last_response_time_ms INTEGER,
    last_http_status INTEGER,
    last_error TEXT,
    config_json TEXT NOT NULL DEFAULT '{}',
    active INTEGER NOT NULL DEFAULT 1,
    first_seen_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    orchestration_group TEXT NOT NULL DEFAULT '',
    orchestration_unit TEXT NOT NULL DEFAULT '',
    UNIQUE(container_name, label_key)
);

INSERT INTO endpoints SELECT * FROM endpoints_backup;
DROP TABLE endpoints_backup;

DROP INDEX IF EXISTS idx_endpoint_source;
