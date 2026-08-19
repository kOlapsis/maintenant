-- Revert ON DELETE CASCADE on containers' child tables (issue #25).

-- 1. Rebuild state_transitions without CASCADE.
CREATE TABLE state_transitions_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id INTEGER NOT NULL REFERENCES containers(id),
    previous_state TEXT NOT NULL,
    new_state TEXT NOT NULL,
    previous_health TEXT,
    new_health TEXT,
    exit_code INTEGER,
    log_snippet TEXT,
    timestamp INTEGER NOT NULL
);

INSERT INTO state_transitions_old (id, container_id, previous_state, new_state,
    previous_health, new_health, exit_code, log_snippet, timestamp)
SELECT id, container_id, previous_state, new_state,
    previous_health, new_health, exit_code, log_snippet, timestamp
FROM state_transitions;

DROP TABLE state_transitions;
ALTER TABLE state_transitions_old RENAME TO state_transitions;

CREATE INDEX IF NOT EXISTS idx_transition_container_time ON state_transitions(container_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_transition_timestamp ON state_transitions(timestamp);

-- 2. Rebuild resource_snapshots without CASCADE.
CREATE TABLE resource_snapshots_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id INTEGER NOT NULL REFERENCES containers(id),
    cpu_percent REAL NOT NULL,
    mem_used INTEGER NOT NULL,
    mem_limit INTEGER NOT NULL,
    net_rx_bytes INTEGER NOT NULL,
    net_tx_bytes INTEGER NOT NULL,
    block_read_bytes INTEGER NOT NULL,
    block_write_bytes INTEGER NOT NULL,
    timestamp INTEGER NOT NULL
);

INSERT INTO resource_snapshots_old (id, container_id, cpu_percent, mem_used, mem_limit,
    net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
SELECT id, container_id, cpu_percent, mem_used, mem_limit,
    net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp
FROM resource_snapshots;

DROP TABLE resource_snapshots;
ALTER TABLE resource_snapshots_old RENAME TO resource_snapshots;

CREATE INDEX IF NOT EXISTS idx_resource_snapshots_container_time ON resource_snapshots(container_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_resource_snapshots_timestamp ON resource_snapshots(timestamp);

-- 3. Rebuild resource_alert_configs without CASCADE.
CREATE TABLE resource_alert_configs_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id INTEGER NOT NULL UNIQUE REFERENCES containers(id),
    cpu_threshold REAL NOT NULL DEFAULT 90.0,
    mem_threshold REAL NOT NULL DEFAULT 90.0,
    enabled INTEGER NOT NULL DEFAULT 0,
    alert_state TEXT NOT NULL DEFAULT 'normal',
    cpu_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
    mem_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
    last_alerted_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO resource_alert_configs_old (id, container_id, cpu_threshold, mem_threshold,
    enabled, alert_state, cpu_consecutive_breaches, mem_consecutive_breaches,
    last_alerted_at, created_at, updated_at)
SELECT id, container_id, cpu_threshold, mem_threshold,
    enabled, alert_state, cpu_consecutive_breaches, mem_consecutive_breaches,
    last_alerted_at, created_at, updated_at
FROM resource_alert_configs;

DROP TABLE resource_alert_configs;
ALTER TABLE resource_alert_configs_old RENAME TO resource_alert_configs;

CREATE INDEX IF NOT EXISTS idx_resource_alert_configs_enabled_state ON resource_alert_configs(enabled, alert_state);