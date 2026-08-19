CREATE TABLE IF NOT EXISTS risk_acknowledgments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  container_external_id TEXT NOT NULL,
  finding_type TEXT NOT NULL,
  finding_key TEXT NOT NULL DEFAULT '',
  acknowledged_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  acknowledged_at INTEGER NOT NULL,
  UNIQUE(container_external_id, finding_type, finding_key)
);

CREATE INDEX idx_risk_ack_container ON risk_acknowledgments(container_external_id);
