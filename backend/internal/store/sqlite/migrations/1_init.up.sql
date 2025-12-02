-- PulseBoard consolidated schema (v1)
-- Merges migrations 1-11 into a single initial schema.

----------------------------------------------------------------------
-- Containers & state transitions
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS containers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('running','exited','completed','restarting','paused','created','dead')),
    health_status TEXT CHECK(health_status IN ('healthy','unhealthy','starting') OR health_status IS NULL),
    has_health_check INTEGER NOT NULL DEFAULT 0,
    orchestration_group TEXT,
    orchestration_unit TEXT,
    custom_group TEXT,
    is_ignored INTEGER NOT NULL DEFAULT 0,
    alert_severity TEXT NOT NULL DEFAULT 'warning' CHECK(alert_severity IN ('critical','warning','info')),
    restart_threshold INTEGER NOT NULL DEFAULT 3,
    alert_channels TEXT,
    archived INTEGER NOT NULL DEFAULT 0,
    first_seen_at INTEGER NOT NULL,
    last_state_change_at INTEGER NOT NULL,
    archived_at INTEGER,
    runtime_type TEXT NOT NULL DEFAULT 'docker',
    error_detail TEXT NOT NULL DEFAULT '',
    controller_kind TEXT NOT NULL DEFAULT '',
    namespace TEXT NOT NULL DEFAULT '',
    pod_count INTEGER NOT NULL DEFAULT 1,
    ready_count INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS state_transitions (
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

CREATE UNIQUE INDEX IF NOT EXISTS idx_container_external_id ON containers(external_id);
CREATE INDEX IF NOT EXISTS idx_transition_container_time ON state_transitions(container_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_transition_timestamp ON state_transitions(timestamp);
CREATE INDEX IF NOT EXISTS idx_container_group ON containers(custom_group, orchestration_group) WHERE archived = 0;
CREATE INDEX IF NOT EXISTS idx_container_archived ON containers(archived, last_state_change_at DESC);

----------------------------------------------------------------------
-- Endpoints & check results
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS endpoints (
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
    UNIQUE(container_name, label_key)
);

CREATE TABLE IF NOT EXISTS check_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint_id INTEGER NOT NULL REFERENCES endpoints(id),
    success INTEGER NOT NULL,
    response_time_ms INTEGER NOT NULL,
    http_status INTEGER,
    error_message TEXT,
    timestamp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_endpoint_identity ON endpoints(container_name, label_key);
CREATE INDEX IF NOT EXISTS idx_endpoint_external_id ON endpoints(external_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_status ON endpoints(status) WHERE active=1;
CREATE INDEX IF NOT EXISTS idx_endpoint_active ON endpoints(active, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_check_endpoint_time ON check_results(endpoint_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_check_timestamp ON check_results(timestamp);

----------------------------------------------------------------------
-- Heartbeats
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS heartbeats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new', 'up', 'down', 'started', 'paused')),
    alert_state TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal', 'alerting')),
    interval_seconds INTEGER NOT NULL,
    grace_seconds INTEGER NOT NULL,
    last_ping_at INTEGER,
    next_deadline_at INTEGER,
    current_run_started_at INTEGER,
    last_exit_code INTEGER,
    last_duration_ms INTEGER,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(uuid)
);

CREATE INDEX IF NOT EXISTS idx_heartbeat_uuid ON heartbeats(uuid);
CREATE INDEX IF NOT EXISTS idx_heartbeat_status_deadline ON heartbeats(status, next_deadline_at) WHERE active=1;
CREATE INDEX IF NOT EXISTS idx_heartbeat_active ON heartbeats(active);

CREATE TABLE IF NOT EXISTS heartbeat_pings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    heartbeat_id INTEGER NOT NULL REFERENCES heartbeats(id),
    ping_type TEXT NOT NULL CHECK(ping_type IN ('success', 'start', 'exit_code')),
    exit_code INTEGER,
    source_ip TEXT NOT NULL,
    http_method TEXT NOT NULL,
    payload TEXT,
    timestamp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hb_ping_heartbeat_time ON heartbeat_pings(heartbeat_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_hb_ping_timestamp ON heartbeat_pings(timestamp);

CREATE TABLE IF NOT EXISTS heartbeat_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    heartbeat_id INTEGER NOT NULL REFERENCES heartbeats(id),
    started_at INTEGER,
    completed_at INTEGER,
    duration_ms INTEGER,
    exit_code INTEGER,
    outcome TEXT NOT NULL CHECK(outcome IN ('success', 'failure', 'timeout', 'in_progress')),
    payload TEXT
);

CREATE INDEX IF NOT EXISTS idx_hb_exec_heartbeat_completed ON heartbeat_executions(heartbeat_id, completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_hb_exec_outcome ON heartbeat_executions(outcome);
CREATE INDEX IF NOT EXISTS idx_hb_exec_completed ON heartbeat_executions(completed_at);

----------------------------------------------------------------------
-- Certificate monitoring
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cert_monitors (
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

CREATE TABLE IF NOT EXISTS cert_check_results (
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

CREATE TABLE IF NOT EXISTS cert_chain_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    check_result_id INTEGER NOT NULL REFERENCES cert_check_results(id),
    position INTEGER NOT NULL,
    subject_cn TEXT NOT NULL,
    issuer_cn TEXT NOT NULL,
    not_before INTEGER NOT NULL,
    not_after INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cert_monitor_identity ON cert_monitors(hostname, port);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cert_monitor_active ON cert_monitors(active, status);
CREATE INDEX IF NOT EXISTS idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone', 'label') AND active = 1;
CREATE INDEX IF NOT EXISTS idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != '';
CREATE INDEX IF NOT EXISTS idx_cert_check_monitor_time ON cert_check_results(monitor_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_cert_check_timestamp ON cert_check_results(checked_at);
CREATE INDEX IF NOT EXISTS idx_chain_entry_check ON cert_chain_entries(check_result_id, position);

----------------------------------------------------------------------
-- Resource monitoring
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS resource_snapshots (
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

CREATE INDEX IF NOT EXISTS idx_resource_snapshots_container_time ON resource_snapshots(container_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_resource_snapshots_timestamp ON resource_snapshots(timestamp);

CREATE TABLE IF NOT EXISTS resource_alert_configs (
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

CREATE INDEX IF NOT EXISTS idx_resource_alert_configs_enabled_state ON resource_alert_configs(enabled, alert_state);

----------------------------------------------------------------------
-- Alert engine
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    alert_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'warning',
    status TEXT NOT NULL DEFAULT 'active',
    message TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    entity_name TEXT NOT NULL,
    details TEXT,
    resolved_by_id INTEGER REFERENCES alerts(id),
    fired_at DATETIME NOT NULL,
    resolved_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_source_severity ON alerts(source, severity);
CREATE INDEX idx_alerts_entity ON alerts(entity_type, entity_id);
CREATE INDEX idx_alerts_fired_at ON alerts(fired_at DESC);
CREATE INDEX idx_alerts_active_dedup ON alerts(source, alert_type, entity_type, entity_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS notification_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL DEFAULT 'webhook',
    url TEXT NOT NULL,
    headers TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS routing_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    source_filter TEXT,
    severity_filter TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_routing_rules_channel ON routing_rules(channel_id);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_deliveries_alert ON notification_deliveries(alert_id);
CREATE INDEX idx_deliveries_channel_status ON notification_deliveries(channel_id, status);

CREATE TABLE IF NOT EXISTS silence_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT,
    entity_id INTEGER,
    source TEXT,
    reason TEXT,
    starts_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    duration_seconds INTEGER NOT NULL,
    cancelled_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_silence_active ON silence_rules(starts_at, duration_seconds, cancelled_at);

----------------------------------------------------------------------
-- Public status page
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS component_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS status_components (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_type TEXT NOT NULL,
    monitor_id INTEGER NOT NULL,
    display_name TEXT NOT NULL,
    group_id INTEGER REFERENCES component_groups(id) ON DELETE SET NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    visible INTEGER NOT NULL DEFAULT 1,
    status_override TEXT,
    auto_incident INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(monitor_type, monitor_id)
);

CREATE INDEX idx_status_components_group_order ON status_components(group_id, display_order);
CREATE INDEX idx_status_components_visible ON status_components(visible);

CREATE TABLE IF NOT EXISTS maintenance_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    starts_at INTEGER NOT NULL,
    ends_at INTEGER NOT NULL,
    active INTEGER NOT NULL DEFAULT 0,
    incident_id INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_maintenance_windows_schedule ON maintenance_windows(starts_at, ends_at);
CREATE INDEX idx_maintenance_windows_active ON maintenance_windows(active);

CREATE TABLE IF NOT EXISTS incidents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'investigating',
    is_maintenance INTEGER NOT NULL DEFAULT 0,
    maintenance_window_id INTEGER REFERENCES maintenance_windows(id),
    created_at INTEGER NOT NULL,
    resolved_at INTEGER,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_incidents_status ON incidents(status);
CREATE INDEX idx_incidents_created_at ON incidents(created_at DESC);

CREATE TABLE IF NOT EXISTS incident_components (
    incident_id INTEGER NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    PRIMARY KEY (incident_id, component_id)
);

CREATE TABLE IF NOT EXISTS incident_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id INTEGER NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT NOT NULL,
    is_auto INTEGER NOT NULL DEFAULT 0,
    alert_id INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_incident_updates_incident_time ON incident_updates(incident_id, created_at);

CREATE TABLE IF NOT EXISTS status_subscribers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    confirmed INTEGER NOT NULL DEFAULT 0,
    confirm_token TEXT UNIQUE,
    confirm_expires INTEGER,
    unsub_token TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS maintenance_components (
    maintenance_id INTEGER NOT NULL REFERENCES maintenance_windows(id) ON DELETE CASCADE,
    component_id INTEGER NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    PRIMARY KEY (maintenance_id, component_id)
);

----------------------------------------------------------------------
-- User & team auth
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id           TEXT NOT NULL PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS teams (
    id          TEXT NOT NULL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    role        TEXT NOT NULL CHECK(role IN ('admin', 'editor', 'viewer')),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS team_memberships (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, team_id)
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    id             TEXT NOT NULL PRIMARY KEY,
    secret_hash    TEXT NOT NULL DEFAULT '',
    grant_types    TEXT NOT NULL DEFAULT '["password","refresh_token"]',
    response_types TEXT NOT NULL DEFAULT '["token"]',
    scopes         TEXT NOT NULL DEFAULT '["read","write","admin"]',
    redirect_uris  TEXT NOT NULL DEFAULT '[]',
    is_public      INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO oauth_clients (id, secret_hash, grant_types, response_types, scopes, redirect_uris, is_public)
VALUES ('pulseboard-ui', '', '["password","refresh_token"]', '["token"]', '["read","write","admin"]', '[]', 1);

CREATE TABLE IF NOT EXISTS oauth_access_tokens (
    signature    TEXT NOT NULL PRIMARY KEY,
    request_id   TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    user_id      TEXT REFERENCES users(id) ON DELETE CASCADE,
    scopes       TEXT NOT NULL DEFAULT '[]',
    granted_scopes TEXT NOT NULL DEFAULT '[]',
    session_data TEXT NOT NULL DEFAULT '{}',
    requested_at DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_oauth_access_tokens_request_id ON oauth_access_tokens(request_id);
CREATE INDEX IF NOT EXISTS idx_oauth_access_tokens_user_id ON oauth_access_tokens(user_id);

CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
    signature    TEXT NOT NULL PRIMARY KEY,
    request_id   TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    user_id      TEXT REFERENCES users(id) ON DELETE CASCADE,
    scopes       TEXT NOT NULL DEFAULT '[]',
    granted_scopes TEXT NOT NULL DEFAULT '[]',
    session_data TEXT NOT NULL DEFAULT '{}',
    requested_at DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_request_id ON oauth_refresh_tokens(request_id);
CREATE INDEX IF NOT EXISTS idx_oauth_refresh_tokens_user_id ON oauth_refresh_tokens(user_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_id    TEXT,
    actor_email TEXT,
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    details     TEXT,
    ip_address  TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);

CREATE TABLE IF NOT EXISTS oauth_client_assertions (
    jti        TEXT NOT NULL PRIMARY KEY,
    expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT NOT NULL PRIMARY KEY,
    value TEXT NOT NULL
);

----------------------------------------------------------------------
-- REST API tokens & webhooks
----------------------------------------------------------------------
CREATE TABLE api_tokens (
    id           TEXT NOT NULL PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    scopes       TEXT NOT NULL DEFAULT '["read"]',
    last_used_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_active    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);

CREATE TABLE webhook_subscriptions (
    id                   TEXT NOT NULL PRIMARY KEY,
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    url                  TEXT NOT NULL,
    secret               TEXT,
    event_types          TEXT NOT NULL DEFAULT '["*"]',
    is_active            INTEGER NOT NULL DEFAULT 1,
    last_delivery_status TEXT,
    last_delivery_at     DATETIME,
    failure_count        INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_webhook_subs_user ON webhook_subscriptions(user_id);

----------------------------------------------------------------------
-- Update intelligence
----------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS image_update_scans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at INTEGER NOT NULL,
    completed_at INTEGER,
    containers_scanned INTEGER NOT NULL DEFAULT 0,
    updates_found INTEGER NOT NULL DEFAULT 0,
    errors INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running'
);

CREATE INDEX IF NOT EXISTS idx_image_update_scans_started_at ON image_update_scans(started_at);

CREATE TABLE IF NOT EXISTS image_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id INTEGER REFERENCES image_update_scans(id),
    container_id TEXT NOT NULL,
    container_name TEXT NOT NULL,
    image TEXT NOT NULL,
    current_tag TEXT NOT NULL,
    current_digest TEXT NOT NULL,
    registry TEXT NOT NULL,
    latest_tag TEXT,
    latest_digest TEXT,
    update_type TEXT,
    published_at INTEGER,
    changelog_url TEXT,
    changelog_summary TEXT,
    has_breaking_changes INTEGER NOT NULL DEFAULT 0,
    risk_score INTEGER NOT NULL DEFAULT 0,
    previous_digest TEXT,
    source_url TEXT,
    status TEXT NOT NULL DEFAULT 'available',
    detected_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_image_updates_container_id ON image_updates(container_id);
CREATE INDEX IF NOT EXISTS idx_image_updates_status ON image_updates(status);
CREATE INDEX IF NOT EXISTS idx_image_updates_detected_at ON image_updates(detected_at);
CREATE INDEX IF NOT EXISTS idx_image_updates_scan_id ON image_updates(scan_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_image_updates_name_image_tag ON image_updates(container_name, image, latest_tag);

CREATE TABLE IF NOT EXISTS cve_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ecosystem TEXT NOT NULL,
    package_name TEXT NOT NULL,
    package_version TEXT NOT NULL,
    cve_id TEXT NOT NULL,
    cvss_score REAL,
    cvss_vector TEXT,
    severity TEXT NOT NULL,
    summary TEXT,
    fixed_in TEXT,
    references_json TEXT,
    fetched_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cve_cache_lookup ON cve_cache(ecosystem, package_name, package_version);
CREATE INDEX IF NOT EXISTS idx_cve_cache_cve_id ON cve_cache(cve_id);
CREATE INDEX IF NOT EXISTS idx_cve_cache_expires_at ON cve_cache(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cve_cache_entry ON cve_cache(ecosystem, package_name, package_version, cve_id);

CREATE TABLE IF NOT EXISTS container_cves (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL,
    cve_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    cvss_score REAL,
    summary TEXT,
    fixed_in TEXT,
    first_detected_at INTEGER NOT NULL,
    resolved_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_container_cves_container_id ON container_cves(container_id);
CREATE INDEX IF NOT EXISTS idx_container_cves_severity ON container_cves(severity);
CREATE UNIQUE INDEX IF NOT EXISTS uq_container_cves ON container_cves(container_id, cve_id);

CREATE TABLE IF NOT EXISTS version_pins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL,
    image TEXT NOT NULL,
    pinned_tag TEXT NOT NULL,
    pinned_digest TEXT NOT NULL,
    reason TEXT,
    pinned_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_version_pins_container ON version_pins(container_id);

CREATE TABLE IF NOT EXISTS update_exclusions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL,
    pattern_type TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_update_exclusions_pattern ON update_exclusions(pattern, pattern_type);

CREATE TABLE IF NOT EXISTS risk_score_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL,
    score INTEGER NOT NULL,
    factors_json TEXT NOT NULL,
    recorded_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_risk_score_history_container_time ON risk_score_history(container_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_risk_score_history_recorded_at ON risk_score_history(recorded_at);
