CREATE TABLE escalation_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT 1,
    active_before_downgrade BOOLEAN NOT NULL DEFAULT 0,
    severities_json TEXT NOT NULL DEFAULT '[]',
    scopes_json TEXT NOT NULL DEFAULT '[]',
    tags_json TEXT NOT NULL DEFAULT '[]',
    levels_json TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by TEXT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT
);
CREATE INDEX idx_escalation_policies_active ON escalation_policies(active);

CREATE TABLE escalation_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    policy_id INTEGER REFERENCES escalation_policies(id) ON DELETE SET NULL,
    policy_snapshot_json TEXT NOT NULL,
    alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN (
        'active','paused_by_maintenance','stopped_by_ack','stopped_by_resolution',
        'stopped_by_policy_deletion','stopped_by_policy_disabled',
        'stopped_by_edition_downgrade','exhausted'
    )),
    last_executed_level_index INTEGER NOT NULL DEFAULT -1,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME,
    next_action_at DATETIME
);
CREATE INDEX idx_escalation_runs_active_due
    ON escalation_runs(status, next_action_at)
    WHERE status = 'active';
CREATE INDEX idx_escalation_runs_alert_id ON escalation_runs(alert_id);
CREATE INDEX idx_escalation_runs_policy_id ON escalation_runs(policy_id);
CREATE INDEX idx_escalation_runs_ended_at ON escalation_runs(ended_at);

CREATE TABLE escalation_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES escalation_runs(id) ON DELETE CASCADE,
    level_index INTEGER NOT NULL,
    channel_id INTEGER REFERENCES notification_channels(id) ON DELETE SET NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','sent','failed','abandoned','skipped_maintenance')),
    error TEXT,
    attempt_started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at DATETIME
);
CREATE UNIQUE INDEX idx_escalation_deliveries_run_level
    ON escalation_deliveries(run_id, level_index, channel_id);
CREATE INDEX idx_escalation_deliveries_pending
    ON escalation_deliveries(status, attempt_started_at)
    WHERE status = 'pending';
CREATE INDEX idx_escalation_deliveries_run_id ON escalation_deliveries(run_id);
