-- Reverts the hard-delete migration: restores the `active` column (defaulting to 1)
-- and drops the ON DELETE CASCADE from child FKs. Purged soft-deleted rows are
-- NOT restored — they are gone.

ALTER TABLE cert_monitors ADD COLUMN active INTEGER NOT NULL DEFAULT 1;

-- Rebuild children to remove ON DELETE CASCADE.
CREATE TABLE cert_check_results_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id INTEGER NOT NULL REFERENCES cert_monitors(id),
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
INSERT INTO cert_check_results_old SELECT * FROM cert_check_results;
DROP TABLE cert_check_results;
ALTER TABLE cert_check_results_old RENAME TO cert_check_results;

CREATE TABLE cert_chain_entries_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    check_result_id INTEGER NOT NULL REFERENCES cert_check_results(id),
    position INTEGER NOT NULL,
    subject_cn TEXT NOT NULL,
    issuer_cn TEXT NOT NULL,
    not_before INTEGER NOT NULL,
    not_after INTEGER NOT NULL
);
INSERT INTO cert_chain_entries_old SELECT * FROM cert_chain_entries;
DROP TABLE cert_chain_entries;
ALTER TABLE cert_chain_entries_old RENAME TO cert_chain_entries;

-- Recreate prior indexes.
CREATE INDEX IF NOT EXISTS idx_cert_monitor_active ON cert_monitors(active, status);
CREATE INDEX IF NOT EXISTS idx_cert_check_monitor_time ON cert_check_results(monitor_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_cert_check_timestamp ON cert_check_results(checked_at);
CREATE INDEX IF NOT EXISTS idx_chain_entry_check ON cert_chain_entries(check_result_id, position);
