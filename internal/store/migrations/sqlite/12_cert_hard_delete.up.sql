-- Remove soft-delete pattern from cert_monitors.
-- Replaces the `active` flag + UNIQUE(hostname,port) workaround with real hard-delete.
-- Also adds ON DELETE CASCADE on cert_check_results/cert_chain_entries so the
-- parent row can go away without orphaning its history.

-- 1. Purge soft-deleted monitors and their dependent data.
DELETE FROM cert_chain_entries
WHERE check_result_id IN (
    SELECT r.id FROM cert_check_results r
    JOIN cert_monitors m ON m.id = r.monitor_id
    WHERE m.active = 0
);
DELETE FROM cert_check_results
WHERE monitor_id IN (SELECT id FROM cert_monitors WHERE active = 0);
DELETE FROM cert_monitors WHERE active = 0;

-- 2. Rebuild cert_monitors without the `active` column.
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
    UNIQUE(hostname, port)
);

INSERT INTO cert_monitors_new (id, hostname, port, source, endpoint_id, status,
    check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
    last_check_at, next_check_at, last_error, created_at, external_id)
SELECT id, hostname, port, source, endpoint_id, status,
    check_interval_seconds, warning_thresholds_json, last_alerted_threshold,
    last_check_at, next_check_at, last_error, created_at, external_id
FROM cert_monitors;

-- 3. Rebuild cert_check_results with ON DELETE CASCADE.
CREATE TABLE cert_check_results_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id INTEGER NOT NULL REFERENCES cert_monitors_new(id) ON DELETE CASCADE,
    subject_cn TEXT,
    issuer_cn TEXT,
    issuer_org TEXT,
    sans_json TEXT,
    serial_number TEXT,
    signature_algorithm TEXT,
    not_before INTEGER,
    not_after INTEGER,
    chain_valid INTEGER,
    chain_error TEXT,
    hostname_match INTEGER,
    error_message TEXT,
    checked_at INTEGER NOT NULL
);

INSERT INTO cert_check_results_new (id, monitor_id, subject_cn, issuer_cn, issuer_org,
    sans_json, serial_number, signature_algorithm, not_before, not_after,
    chain_valid, chain_error, hostname_match, error_message, checked_at)
SELECT id, monitor_id, subject_cn, issuer_cn, issuer_org,
    sans_json, serial_number, signature_algorithm, not_before, not_after,
    chain_valid, chain_error, hostname_match, error_message, checked_at
FROM cert_check_results;

-- 4. Rebuild cert_chain_entries with ON DELETE CASCADE.
CREATE TABLE cert_chain_entries_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    check_result_id INTEGER NOT NULL REFERENCES cert_check_results_new(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    subject_cn TEXT NOT NULL,
    issuer_cn TEXT NOT NULL,
    not_before INTEGER NOT NULL,
    not_after INTEGER NOT NULL
);

INSERT INTO cert_chain_entries_new (id, check_result_id, position, subject_cn, issuer_cn, not_before, not_after)
SELECT id, check_result_id, position, subject_cn, issuer_cn, not_before, not_after
FROM cert_chain_entries;

-- 5. Swap old -> new.
DROP TABLE cert_chain_entries;
DROP TABLE cert_check_results;
DROP TABLE cert_monitors;
ALTER TABLE cert_monitors_new RENAME TO cert_monitors;
ALTER TABLE cert_check_results_new RENAME TO cert_check_results;
ALTER TABLE cert_chain_entries_new RENAME TO cert_chain_entries;

-- 6. Recreate indexes (no more active-filtered variants).
CREATE UNIQUE INDEX IF NOT EXISTS idx_cert_monitor_identity ON cert_monitors(hostname, port);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cert_monitor_status ON cert_monitors(status);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone', 'label');
CREATE INDEX IF NOT EXISTS idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != '';
CREATE INDEX IF NOT EXISTS idx_cert_check_monitor_time ON cert_check_results(monitor_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_cert_check_timestamp ON cert_check_results(checked_at);
CREATE INDEX IF NOT EXISTS idx_chain_entry_check ON cert_chain_entries(check_result_id, position);
