-- Make label-discovered endpoint / TLS-cert monitor identity agent-aware.
--
-- Remote agents push monitors observed on their OWN host. Two agents may each
-- watch "localhost:443", or each run a container named "web" exposing the same
-- maintenant.endpoint.* label. The pre-existing global UNIQUE(hostname,port)
-- (cert_monitors) and UNIQUE(container_name,label_key) (endpoints) made those
-- collide into a single row. We relax identity to include COALESCE(agent_id,''):
--   * local rows (agent_id IS NULL) collapse to '' and keep their original dedup,
--   * each remote agent gets an independent namespace.
--
-- A table-level UNIQUE constraint cannot be dropped in place in SQLite, and a
-- UNIQUE constraint cannot express COALESCE(); both require a table rebuild plus
-- a UNIQUE INDEX. The rebuild runs with foreign_keys OFF so inbound references
-- survive the drop+rename:
--   endpoints(id)    <- check_results.endpoint_id, cert_monitors.endpoint_id
--   cert_monitors(id) <- cert_check_results.monitor_id

PRAGMA foreign_keys=OFF;
PRAGMA legacy_alter_table=ON;

-- ---------------------------------------------------------------- endpoints ---
CREATE TABLE endpoints_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_name TEXT NOT NULL,
    label_key TEXT NOT NULL,
    external_id TEXT NOT NULL,
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
    source TEXT NOT NULL DEFAULT 'label' CHECK(source IN ('label', 'standalone')),
    name TEXT NOT NULL DEFAULT '',
    agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE
);
INSERT INTO endpoints_new (
    id, container_name, label_key, external_id, endpoint_type, target, status,
    alert_state, consecutive_failures, consecutive_successes, last_check_at,
    last_response_time_ms, last_http_status, last_error, config_json, active,
    first_seen_at, last_seen_at, source, name, agent_id)
SELECT
    id, container_name, label_key, external_id, endpoint_type, target, status,
    alert_state, consecutive_failures, consecutive_successes, last_check_at,
    last_response_time_ms, last_http_status, last_error, config_json, active,
    first_seen_at, last_seen_at, source, name, agent_id
FROM endpoints;
DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- Identity is now UNIQUE per (container_name, label_key, agent). Was a non-unique
-- index + a table-level UNIQUE(container_name, label_key); folded into one.
CREATE UNIQUE INDEX idx_endpoint_identity ON endpoints(container_name, label_key, COALESCE(agent_id, ''));
CREATE INDEX idx_endpoint_external_id ON endpoints(external_id);
CREATE INDEX idx_endpoint_status ON endpoints(status) WHERE active=1;
CREATE INDEX idx_endpoint_active ON endpoints(active, last_seen_at DESC);
CREATE INDEX idx_endpoint_source ON endpoints(source) WHERE active=1;
CREATE INDEX idx_endpoints_agent_id ON endpoints(agent_id) WHERE agent_id IS NOT NULL;

-- ------------------------------------------------------------ cert_monitors ---
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
    created_at INTEGER NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT REFERENCES agents(agent_id) ON DELETE CASCADE
);
INSERT INTO cert_monitors_new (
    id, hostname, port, source, endpoint_id, status, check_interval_seconds,
    warning_thresholds_json, last_alerted_threshold, last_check_at, next_check_at,
    last_error, created_at, external_id, agent_id)
SELECT
    id, hostname, port, source, endpoint_id, status, check_interval_seconds,
    warning_thresholds_json, last_alerted_threshold, last_check_at, next_check_at,
    last_error, created_at, external_id, agent_id
FROM cert_monitors;
DROP TABLE cert_monitors;
ALTER TABLE cert_monitors_new RENAME TO cert_monitors;

-- Identity is now UNIQUE per (hostname, port, agent). Was table-level
-- UNIQUE(hostname, port) + a redundant unique index.
CREATE UNIQUE INDEX idx_cert_monitor_identity ON cert_monitors(hostname, port, COALESCE(agent_id, ''));
CREATE INDEX idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX idx_cert_monitor_status ON cert_monitors(status);
CREATE INDEX idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone', 'label');
CREATE INDEX idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != '';
CREATE INDEX idx_cert_monitors_agent_id ON cert_monitors(agent_id) WHERE agent_id IS NOT NULL;

PRAGMA legacy_alter_table=OFF;
PRAGMA foreign_keys=ON;
