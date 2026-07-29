-- maintenant — UUID-native, portable (SQLite/Postgres) schema (target).
-- Conventions:
--   * id TEXT PRIMARY KEY (UUID) everywhere except join tables (composite PK)
--     and natural-key tables (role / hash / token).
--   * <x>_id TEXT FK. agent_id TEXT NOT NULL DEFAULT sentinel, FK agents(id).
--   * timestamps: BIGINT epoch seconds (no DATETIME, no DEFAULT CURRENT_TIMESTAMP).
--   * booleans: INTEGER 0/1. binary: BLOB. No AUTOINCREMENT.
--   * upserts use ON CONFLICT (handled in Go); UNIQUE constraints are plain.
-- Applied by the Go UUID transform (FK enforcement is managed there), so this
-- file intentionally carries no PRAGMA.

-- =============================================================== agents ======
CREATE TABLE agents (
    id                TEXT PRIMARY KEY NOT NULL,            -- UUID (agent-generated) or LocalAgent sentinel
    public_key        BLOB,                                 -- raw 32 bytes Ed25519; NULL for the local sentinel
    hostname          TEXT NOT NULL,
    label             TEXT NOT NULL DEFAULT '',
    os_arch           TEXT NOT NULL DEFAULT '',
    agent_version     TEXT NOT NULL DEFAULT '',
    detected_runtime  TEXT NOT NULL DEFAULT 'local' CHECK (detected_runtime IN ('docker','swarm','kubernetes','local')),
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    last_seen_at      BIGINT,
    created_at        BIGINT NOT NULL DEFAULT 0,
    revoked_at        BIGINT,
    revoked_by        TEXT,
    CHECK (length(label) <= 64)
);
CREATE INDEX idx_agents_status       ON agents(status);
CREATE INDEX idx_agents_last_seen_at ON agents(last_seen_at);

-- The local sentinel agent: attributes every entity discovered by the server's
-- own in-process runtime, so agent_id is never NULL.
INSERT INTO agents (id, public_key, hostname, label, os_arch, agent_version, detected_runtime, status, created_at)
VALUES ('00000000-0000-0000-0000-000000000000', NULL, 'local', 'local', '', '', 'local', 'active', 0);

CREATE TABLE enrollment_tokens (
    id            TEXT PRIMARY KEY NOT NULL,                -- hex(sha256(token))[:16]
    token         TEXT NOT NULL UNIQUE,
    created_at    BIGINT NOT NULL DEFAULT 0,
    expires_at    BIGINT NOT NULL,
    consumed_at   BIGINT,
    consumed_by_agent_id TEXT                               -- audit only, no FK (agent may not exist yet)
);
CREATE INDEX idx_enrollment_tokens_expires_at ON enrollment_tokens(expires_at);

-- =========================================================== containers ======
CREATE TABLE containers (
    id                   TEXT PRIMARY KEY NOT NULL,         -- uid.Container(agent_id, external_id)
    agent_id             TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    external_id          TEXT NOT NULL,
    name                 TEXT NOT NULL,
    image                TEXT NOT NULL,
    state                TEXT NOT NULL CHECK(state IN ('running','exited','completed','restarting','paused','created','dead')),
    health_status        TEXT CHECK(health_status IN ('healthy','unhealthy','starting') OR health_status IS NULL),
    has_health_check     INTEGER NOT NULL DEFAULT 0,
    orchestration_group  TEXT,
    orchestration_unit   TEXT,
    custom_group         TEXT,
    is_ignored           INTEGER NOT NULL DEFAULT 0,
    alert_severity       TEXT NOT NULL DEFAULT 'warning' CHECK(alert_severity IN ('critical','warning','info')),
    restart_threshold    INTEGER NOT NULL DEFAULT 3,
    alert_channels       TEXT,
    archived             INTEGER NOT NULL DEFAULT 0,
    first_seen_at        BIGINT NOT NULL,
    last_state_change_at BIGINT NOT NULL,
    archived_at          BIGINT,
    runtime_type         TEXT NOT NULL DEFAULT 'docker',
    error_detail         TEXT NOT NULL DEFAULT '',
    controller_kind      TEXT NOT NULL DEFAULT '',
    namespace            TEXT NOT NULL DEFAULT '',
    pod_count            INTEGER NOT NULL DEFAULT 1,
    ready_count          INTEGER NOT NULL DEFAULT 1,
    compose_working_dir  TEXT NOT NULL DEFAULT '',
    swarm_service_id     TEXT NOT NULL DEFAULT '',
    swarm_service_name   TEXT NOT NULL DEFAULT '',
    swarm_service_mode   TEXT NOT NULL DEFAULT '',
    swarm_node_id        TEXT NOT NULL DEFAULT '',
    swarm_task_slot      INTEGER NOT NULL DEFAULT 0,
    swarm_desired_replicas INTEGER NOT NULL DEFAULT 0,
    UNIQUE(agent_id, external_id)
);
CREATE INDEX idx_containers_agent_id ON containers(agent_id);
CREATE INDEX idx_container_group ON containers(custom_group, orchestration_group) WHERE archived = 0;
CREATE INDEX idx_container_archived ON containers(archived, last_state_change_at DESC);
CREATE INDEX idx_containers_swarm_service ON containers(swarm_service_id) WHERE swarm_service_id != '';

CREATE TABLE state_transitions (
    id              TEXT PRIMARY KEY NOT NULL,
    container_id    TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    previous_state  TEXT NOT NULL,
    new_state       TEXT NOT NULL,
    previous_health TEXT,
    new_health      TEXT,
    exit_code       INTEGER,
    log_snippet     TEXT,
    timestamp       BIGINT NOT NULL
);
CREATE INDEX idx_transition_container_time ON state_transitions(container_id, timestamp DESC);
CREATE INDEX idx_transition_timestamp ON state_transitions(timestamp);

CREATE TABLE resource_snapshots (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    cpu_percent   REAL NOT NULL,
    mem_used      BIGINT NOT NULL,
    mem_limit     BIGINT NOT NULL,
    net_rx_bytes  BIGINT NOT NULL,
    net_tx_bytes  BIGINT NOT NULL,
    block_read_bytes  BIGINT NOT NULL,
    block_write_bytes BIGINT NOT NULL,
    timestamp     BIGINT NOT NULL
);
CREATE INDEX idx_resource_snapshots_container_time ON resource_snapshots(container_id, timestamp DESC);
CREATE INDEX idx_resource_snapshots_timestamp ON resource_snapshots(timestamp);
CREATE INDEX idx_resource_snapshots_agent_id ON resource_snapshots(agent_id);

CREATE TABLE resource_alert_configs (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL UNIQUE REFERENCES containers(id) ON DELETE CASCADE,
    cpu_threshold REAL NOT NULL DEFAULT 90.0,
    mem_threshold REAL NOT NULL DEFAULT 90.0,
    enabled       INTEGER NOT NULL DEFAULT 0,
    alert_state   TEXT NOT NULL DEFAULT 'normal',
    cpu_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
    mem_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
    last_alerted_at BIGINT,
    created_at    BIGINT NOT NULL,
    updated_at    BIGINT NOT NULL
);
CREATE INDEX idx_resource_alert_configs_enabled_state ON resource_alert_configs(enabled, alert_state);

CREATE TABLE resource_hourly (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL,
    bucket        BIGINT NOT NULL,
    avg_cpu_percent REAL NOT NULL,
    avg_mem_used  BIGINT NOT NULL,
    avg_mem_limit BIGINT NOT NULL,
    avg_net_rx_bytes BIGINT NOT NULL,
    avg_net_tx_bytes BIGINT NOT NULL,
    avg_block_read_bytes BIGINT NOT NULL DEFAULT 0,
    avg_block_write_bytes BIGINT NOT NULL DEFAULT 0,
    sample_count  INTEGER NOT NULL,
    UNIQUE(container_id, bucket)
);
CREATE INDEX idx_resource_hourly_lookup ON resource_hourly(container_id, bucket);

CREATE TABLE resource_daily (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL,
    bucket        BIGINT NOT NULL,
    avg_cpu_percent REAL NOT NULL,
    avg_mem_used  BIGINT NOT NULL,
    avg_mem_limit BIGINT NOT NULL,
    avg_net_rx_bytes BIGINT NOT NULL,
    avg_net_tx_bytes BIGINT NOT NULL,
    sample_count  INTEGER NOT NULL,
    UNIQUE(container_id, bucket)
);
CREATE INDEX idx_resource_daily_lookup ON resource_daily(container_id, bucket);

-- ============================================================ endpoints ======
CREATE TABLE endpoints (
    id              TEXT PRIMARY KEY NOT NULL,              -- uid.EndpointLabel(agent,container,label) or minted (standalone)
    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    container_name  TEXT NOT NULL,
    label_key       TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    endpoint_type   TEXT NOT NULL CHECK(endpoint_type IN ('http','tcp')),
    target          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('up','down','degraded','unknown')),
    alert_state     TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal','alerting')),
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_check_at         BIGINT,
    last_response_time_ms BIGINT,
    last_http_status      INTEGER,
    last_error      TEXT,
    config_json     TEXT NOT NULL DEFAULT '{}',
    active          INTEGER NOT NULL DEFAULT 1,
    first_seen_at   BIGINT NOT NULL,
    last_seen_at    BIGINT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'label' CHECK(source IN ('label','standalone')),
    name            TEXT NOT NULL DEFAULT '',
    UNIQUE(agent_id, container_name, label_key)
);
CREATE INDEX idx_endpoint_external_id ON endpoints(external_id);
CREATE INDEX idx_endpoint_status ON endpoints(status) WHERE active=1;
CREATE INDEX idx_endpoint_active ON endpoints(active, last_seen_at DESC);
CREATE INDEX idx_endpoint_source ON endpoints(source) WHERE active=1;
CREATE INDEX idx_endpoints_agent_id ON endpoints(agent_id);

CREATE TABLE check_results (
    id            TEXT PRIMARY KEY NOT NULL,
    endpoint_id   TEXT NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    success       INTEGER NOT NULL,
    response_time_ms INTEGER NOT NULL,
    http_status   INTEGER,
    error_message TEXT,
    timestamp     BIGINT NOT NULL
);
CREATE INDEX idx_check_endpoint_time ON check_results(endpoint_id, timestamp DESC);
CREATE INDEX idx_check_timestamp ON check_results(timestamp);

-- ========================================================= cert monitors =====
CREATE TABLE cert_monitors (
    id              TEXT PRIMARY KEY NOT NULL,              -- uid.CertMonitor(agent,host,port[,server_name]) or minted (standalone)
    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    hostname        TEXT NOT NULL,
    port            INTEGER NOT NULL DEFAULT 443,
    source          TEXT NOT NULL CHECK(source IN ('auto','standalone','label')),
    endpoint_id     TEXT REFERENCES endpoints(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('valid','expiring','expired','error','unknown')),
    check_interval_seconds INTEGER NOT NULL DEFAULT 43200,
    warning_thresholds_json TEXT NOT NULL DEFAULT '[30,14,7,3,1]',
    last_alerted_threshold INTEGER,
    last_check_at   BIGINT,
    next_check_at   BIGINT,
    last_error      TEXT,
    created_at      BIGINT NOT NULL,
    external_id     TEXT NOT NULL DEFAULT '',
    server_name     TEXT NOT NULL DEFAULT '',               -- SNI; '' = validate against hostname
    UNIQUE(agent_id, hostname, port, server_name)
);
CREATE INDEX idx_cert_monitor_endpoint ON cert_monitors(endpoint_id) WHERE endpoint_id IS NOT NULL;
CREATE INDEX idx_cert_monitor_status ON cert_monitors(status);
CREATE INDEX idx_cert_monitor_next_check ON cert_monitors(next_check_at) WHERE source IN ('standalone','label');
CREATE INDEX idx_cert_monitor_external_id ON cert_monitors(external_id) WHERE external_id != '';
CREATE INDEX idx_cert_monitors_agent_id ON cert_monitors(agent_id);

CREATE TABLE cert_check_results (
    id            TEXT PRIMARY KEY NOT NULL,
    monitor_id    TEXT NOT NULL REFERENCES cert_monitors(id) ON DELETE CASCADE,
    subject_cn    TEXT,
    issuer_cn     TEXT,
    issuer_org    TEXT,
    sans_json     TEXT,
    serial_number TEXT,
    signature_algorithm TEXT,
    not_before    BIGINT,
    not_after     BIGINT,
    chain_valid   INTEGER,
    chain_error   TEXT,
    hostname_match INTEGER,
    error_message TEXT,
    checked_at    BIGINT NOT NULL,
    ocsp_stapled  INTEGER,
    ocsp_status   TEXT,
    ocsp_produced_at BIGINT,
    ocsp_next_update BIGINT,
    ocsp_error    TEXT
);
CREATE INDEX idx_cert_check_monitor_time ON cert_check_results(monitor_id, checked_at DESC);
CREATE INDEX idx_cert_check_timestamp ON cert_check_results(checked_at);

CREATE TABLE cert_chain_entries (
    id              TEXT PRIMARY KEY NOT NULL,
    check_result_id TEXT NOT NULL REFERENCES cert_check_results(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL,
    subject_cn      TEXT NOT NULL,
    issuer_cn       TEXT NOT NULL,
    not_before      BIGINT NOT NULL,
    not_after       BIGINT NOT NULL
);
CREATE INDEX idx_chain_entry_check ON cert_chain_entries(check_result_id, position);

-- ============================================================ heartbeats =====
-- PK is the heartbeat's public ping token (already a UUID). No separate int id.
CREATE TABLE heartbeats (
    id              TEXT PRIMARY KEY NOT NULL,              -- the ping token (was column "uuid")
    agent_id        TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','up','down','started','paused')),
    alert_state     TEXT NOT NULL DEFAULT 'normal' CHECK(alert_state IN ('normal','alerting')),
    interval_seconds INTEGER NOT NULL,
    grace_seconds   INTEGER NOT NULL,
    last_ping_at    BIGINT,
    next_deadline_at BIGINT,
    current_run_started_at BIGINT,
    last_exit_code  INTEGER,
    last_duration_ms BIGINT,
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    active          INTEGER NOT NULL DEFAULT 1,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);
CREATE INDEX idx_heartbeat_status_deadline ON heartbeats(status, next_deadline_at) WHERE active=1;
CREATE INDEX idx_heartbeat_active ON heartbeats(active);
CREATE INDEX idx_heartbeats_agent_id ON heartbeats(agent_id);

CREATE TABLE heartbeat_pings (
    id            TEXT PRIMARY KEY NOT NULL,
    heartbeat_id  TEXT NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    ping_type     TEXT NOT NULL CHECK(ping_type IN ('success','start','exit_code')),
    exit_code     INTEGER,
    source_ip     TEXT NOT NULL,
    http_method   TEXT NOT NULL,
    payload       TEXT,
    timestamp     BIGINT NOT NULL
);
CREATE INDEX idx_hb_ping_heartbeat_time ON heartbeat_pings(heartbeat_id, timestamp DESC);
CREATE INDEX idx_hb_ping_timestamp ON heartbeat_pings(timestamp);

CREATE TABLE heartbeat_executions (
    id            TEXT PRIMARY KEY NOT NULL,
    heartbeat_id  TEXT NOT NULL REFERENCES heartbeats(id) ON DELETE CASCADE,
    started_at    BIGINT,
    completed_at  BIGINT,
    duration_ms   BIGINT,
    exit_code     INTEGER,
    outcome       TEXT NOT NULL CHECK(outcome IN ('success','failure','timeout','in_progress')),
    payload       TEXT
);
CREATE INDEX idx_hb_exec_heartbeat_completed ON heartbeat_executions(heartbeat_id, completed_at DESC);
CREATE INDEX idx_hb_exec_outcome ON heartbeat_executions(outcome);
CREATE INDEX idx_hb_exec_completed ON heartbeat_executions(completed_at);

-- ================================================================ alerts =====
-- entity_id is a polymorphic reference (by entity_type) to a container/endpoint/
-- cert_monitor/heartbeat id — now a TEXT UUID. Remapped per-type in the transform.
CREATE TABLE alerts (
    id            TEXT PRIMARY KEY NOT NULL,
    source        TEXT NOT NULL,
    alert_type    TEXT NOT NULL,
    severity      TEXT NOT NULL DEFAULT 'warning',
    status        TEXT NOT NULL DEFAULT 'active',
    message       TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    entity_name   TEXT NOT NULL,
    details       TEXT,
    resolved_by_id TEXT REFERENCES alerts(id) ON DELETE SET NULL,
    fired_at      BIGINT NOT NULL,
    resolved_at   BIGINT,
    created_at    BIGINT NOT NULL DEFAULT 0,
    acknowledged_at BIGINT,
    acknowledged_by TEXT,
    escalated_at  BIGINT
);
CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_source_severity ON alerts(source, severity);
CREATE INDEX idx_alerts_entity ON alerts(entity_type, entity_id);
CREATE INDEX idx_alerts_fired_at ON alerts(fired_at DESC);
CREATE INDEX idx_alerts_active_dedup ON alerts(source, alert_type, entity_type, entity_id) WHERE status = 'active';

CREATE TABLE notification_channels (
    id            TEXT PRIMARY KEY NOT NULL,
    name          TEXT NOT NULL UNIQUE,
    type          TEXT NOT NULL DEFAULT 'webhook',
    url           TEXT NOT NULL,
    headers       TEXT,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE notification_deliveries (
    id            TEXT PRIMARY KEY NOT NULL,
    alert_id      TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    channel_id    TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending',
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT,
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_deliveries_alert ON notification_deliveries(alert_id);
CREATE INDEX idx_deliveries_channel_status ON notification_deliveries(channel_id, status);

CREATE TABLE silence_rules (
    id            TEXT PRIMARY KEY NOT NULL,
    entity_type   TEXT,
    entity_id     TEXT,
    source        TEXT,
    reason        TEXT,
    starts_at     BIGINT NOT NULL DEFAULT 0,
    duration_seconds INTEGER NOT NULL,
    cancelled_at  BIGINT,
    created_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_silence_active ON silence_rules(starts_at, duration_seconds, cancelled_at);

-- ========================================================= alert routing =====
CREATE TABLE alert_triggers (
    id            TEXT PRIMARY KEY NOT NULL,
    name          TEXT NOT NULL UNIQUE,
    filter_severities TEXT NOT NULL DEFAULT '',
    filter_sources    TEXT NOT NULL DEFAULT '',
    filter_scopes     TEXT NOT NULL DEFAULT '',
    filter_tags       TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_alert_triggers_enabled ON alert_triggers(enabled);

CREATE TABLE alert_trigger_channels (
    trigger_id    TEXT NOT NULL REFERENCES alert_triggers(id) ON DELETE CASCADE,
    channel_id    TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY(trigger_id, channel_id)
);
CREATE INDEX idx_atc_channel ON alert_trigger_channels(channel_id);

-- ============================================================ escalation =====
CREATE TABLE escalation_policies (
    id            TEXT PRIMARY KEY NOT NULL,
    name          TEXT NOT NULL,
    active        INTEGER NOT NULL DEFAULT 1,
    active_before_downgrade INTEGER NOT NULL DEFAULT 0,
    severities_json TEXT NOT NULL DEFAULT '[]',
    scopes_json   TEXT NOT NULL DEFAULT '[]',
    tags_json     TEXT NOT NULL DEFAULT '[]',
    levels_json   TEXT NOT NULL,
    created_at    BIGINT NOT NULL DEFAULT 0,
    created_by    TEXT,
    updated_at    BIGINT NOT NULL DEFAULT 0,
    updated_by    TEXT
);
CREATE INDEX idx_escalation_policies_active ON escalation_policies(active);

CREATE TABLE escalation_runs (
    id            TEXT PRIMARY KEY NOT NULL,
    policy_id     TEXT REFERENCES escalation_policies(id) ON DELETE SET NULL,
    policy_snapshot_json TEXT NOT NULL,
    alert_id      TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    status        TEXT NOT NULL CHECK(status IN ('active','paused_by_maintenance','stopped_by_ack','stopped_by_resolution','stopped_by_policy_deletion','stopped_by_policy_disabled','stopped_by_edition_downgrade','exhausted')),
    last_executed_level_index INTEGER NOT NULL DEFAULT -1,
    started_at    BIGINT NOT NULL DEFAULT 0,
    ended_at      BIGINT,
    next_action_at BIGINT
);
CREATE INDEX idx_escalation_runs_active_due ON escalation_runs(status, next_action_at) WHERE status = 'active';
CREATE INDEX idx_escalation_runs_alert_id ON escalation_runs(alert_id);
CREATE INDEX idx_escalation_runs_policy_id ON escalation_runs(policy_id);
CREATE INDEX idx_escalation_runs_ended_at ON escalation_runs(ended_at);

CREATE TABLE escalation_deliveries (
    id            TEXT PRIMARY KEY NOT NULL,
    run_id        TEXT NOT NULL REFERENCES escalation_runs(id) ON DELETE CASCADE,
    level_index   INTEGER NOT NULL,
    channel_id    TEXT REFERENCES notification_channels(id) ON DELETE SET NULL,
    status        TEXT NOT NULL CHECK(status IN ('pending','sent','failed','abandoned','skipped_maintenance')),
    error         TEXT,
    attempt_started_at BIGINT NOT NULL DEFAULT 0,
    sent_at       BIGINT
);
CREATE UNIQUE INDEX idx_escalation_deliveries_run_level ON escalation_deliveries(run_id, level_index, channel_id);
CREATE INDEX idx_escalation_deliveries_pending ON escalation_deliveries(status, attempt_started_at) WHERE status = 'pending';
CREATE INDEX idx_escalation_deliveries_run_id ON escalation_deliveries(run_id);

-- ====================================================== status / incidents ===
CREATE TABLE status_components (
    id              TEXT PRIMARY KEY NOT NULL,
    composition_mode TEXT NOT NULL DEFAULT 'explicit',
    match_all_type  TEXT,
    display_name    TEXT NOT NULL,
    display_order   INTEGER NOT NULL DEFAULT 0,
    visible         INTEGER NOT NULL DEFAULT 1,
    status_override TEXT,
    auto_incident   INTEGER NOT NULL DEFAULT 0,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);
CREATE INDEX idx_status_components_visible ON status_components(visible);

-- monitor_id is polymorphic by monitor_type (endpoint/container/cert/heartbeat),
-- now a TEXT UUID. Remapped per-type in the transform.
CREATE TABLE status_component_monitors (
    component_id  TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    monitor_type  TEXT NOT NULL,
    monitor_id    TEXT NOT NULL,
    PRIMARY KEY(component_id, monitor_type, monitor_id)
);
CREATE INDEX idx_status_component_monitors_lookup ON status_component_monitors(monitor_type, monitor_id);

CREATE TABLE incidents (
    id            TEXT PRIMARY KEY NOT NULL,
    title         TEXT NOT NULL,
    severity      TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'investigating',
    is_maintenance INTEGER NOT NULL DEFAULT 0,
    maintenance_window_id TEXT,
    created_at    BIGINT NOT NULL,
    resolved_at   BIGINT,
    updated_at    BIGINT NOT NULL
);
CREATE INDEX idx_incidents_status ON incidents(status);
CREATE INDEX idx_incidents_created_at ON incidents(created_at DESC);

CREATE TABLE incident_components (
    incident_id   TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    component_id  TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    PRIMARY KEY(incident_id, component_id)
);

CREATE TABLE incident_updates (
    id            TEXT PRIMARY KEY NOT NULL,
    incident_id   TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    status        TEXT NOT NULL,
    message       TEXT NOT NULL,
    is_auto       INTEGER NOT NULL DEFAULT 0,
    alert_id      TEXT,
    created_at    BIGINT NOT NULL
);
CREATE INDEX idx_incident_updates_incident_time ON incident_updates(incident_id, created_at);

CREATE TABLE maintenance_windows (
    id            TEXT PRIMARY KEY NOT NULL,
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    starts_at     BIGINT NOT NULL,
    ends_at       BIGINT NOT NULL,
    active        INTEGER NOT NULL DEFAULT 0,
    incident_id   TEXT,
    created_at    BIGINT NOT NULL,
    updated_at    BIGINT NOT NULL
);
CREATE INDEX idx_maintenance_windows_schedule ON maintenance_windows(starts_at, ends_at);
CREATE INDEX idx_maintenance_windows_active ON maintenance_windows(active);

CREATE TABLE maintenance_components (
    maintenance_id TEXT NOT NULL REFERENCES maintenance_windows(id) ON DELETE CASCADE,
    component_id   TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    PRIMARY KEY(maintenance_id, component_id)
);

CREATE TABLE status_subscribers (
    id            TEXT PRIMARY KEY NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    confirmed     INTEGER NOT NULL DEFAULT 0,
    confirm_token TEXT UNIQUE,
    confirm_expires BIGINT,
    unsub_token   TEXT NOT NULL UNIQUE,
    created_at    BIGINT NOT NULL
);

-- ================================================ status page personalization
CREATE TABLE status_page_settings (
    id            INTEGER PRIMARY KEY CHECK(id = 1),
    version       INTEGER NOT NULL DEFAULT 0,
    title         TEXT NOT NULL DEFAULT 'System Status',
    subtitle      TEXT NOT NULL DEFAULT '',
    color_bg      TEXT NOT NULL DEFAULT '#0B0E13',
    color_surface TEXT NOT NULL DEFAULT '#12151C',
    color_border  TEXT NOT NULL DEFAULT '#1F2937',
    color_text    TEXT NOT NULL DEFAULT '#FFFFFF',
    color_accent  TEXT NOT NULL DEFAULT '#22C55E',
    color_status_operational TEXT NOT NULL DEFAULT '#22C55E',
    color_status_degraded    TEXT NOT NULL DEFAULT '#EAB308',
    color_status_partial     TEXT NOT NULL DEFAULT '#F97316',
    color_status_major       TEXT NOT NULL DEFAULT '#EF4444',
    announcement_enabled INTEGER NOT NULL DEFAULT 0,
    announcement_message_md   TEXT NOT NULL DEFAULT '',
    announcement_message_html TEXT NOT NULL DEFAULT '',
    announcement_url  TEXT NOT NULL DEFAULT '',
    footer_text_md    TEXT NOT NULL DEFAULT '',
    footer_text_html  TEXT NOT NULL DEFAULT '',
    locale        TEXT NOT NULL DEFAULT 'en',
    timezone      TEXT NOT NULL DEFAULT '',
    date_format   TEXT NOT NULL DEFAULT 'relative',
    updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE status_page_assets (
    role          TEXT PRIMARY KEY CHECK(role IN ('logo','favicon','hero')),
    mime          TEXT NOT NULL,
    bytes         BLOB NOT NULL,
    byte_size     INTEGER NOT NULL,
    alt_text      TEXT NOT NULL DEFAULT '',
    updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE status_page_footer_links (
    id            TEXT PRIMARY KEY NOT NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    label         TEXT NOT NULL DEFAULT '',
    url           TEXT NOT NULL,
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_status_page_footer_links_position ON status_page_footer_links(position);

CREATE TABLE status_page_faq_items (
    id            TEXT PRIMARY KEY NOT NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    question      TEXT NOT NULL,
    answer_md     TEXT NOT NULL,
    answer_html   TEXT NOT NULL,
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_status_page_faq_items_position ON status_page_faq_items(position);

-- ============================================ update intelligence / CVE ======
-- container_id here is the container external_id (Docker id), a soft natural
-- reference (no FK), kept as TEXT — unchanged from current semantics.
CREATE TABLE image_update_scans (
    id            TEXT PRIMARY KEY NOT NULL,
    started_at    BIGINT NOT NULL,
    completed_at  BIGINT,
    containers_scanned INTEGER NOT NULL DEFAULT 0,
    updates_found INTEGER NOT NULL DEFAULT 0,
    errors        INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'running'
);
CREATE INDEX idx_image_update_scans_started_at ON image_update_scans(started_at);

CREATE TABLE image_updates (
    id            TEXT PRIMARY KEY NOT NULL,
    scan_id       TEXT REFERENCES image_update_scans(id) ON DELETE SET NULL,
    container_id  TEXT NOT NULL,
    container_name TEXT NOT NULL,
    image         TEXT NOT NULL,
    current_tag   TEXT NOT NULL,
    current_digest TEXT NOT NULL,
    registry      TEXT NOT NULL,
    latest_tag    TEXT,
    latest_digest TEXT,
    update_type   TEXT,
    published_at  BIGINT,
    changelog_url TEXT,
    changelog_summary TEXT,
    has_breaking_changes INTEGER NOT NULL DEFAULT 0,
    risk_score    INTEGER NOT NULL DEFAULT 0,
    previous_digest TEXT,
    source_url    TEXT,
    status        TEXT NOT NULL DEFAULT 'available',
    detected_at   BIGINT NOT NULL
);
CREATE INDEX idx_image_updates_container_id ON image_updates(container_id);
CREATE INDEX idx_image_updates_status ON image_updates(status);
CREATE INDEX idx_image_updates_detected_at ON image_updates(detected_at);
CREATE INDEX idx_image_updates_scan_id ON image_updates(scan_id);
CREATE UNIQUE INDEX uq_image_updates_name_image_tag ON image_updates(container_name, image, latest_tag);

CREATE TABLE cve_cache (
    id            TEXT PRIMARY KEY NOT NULL,
    ecosystem     TEXT NOT NULL,
    package_name  TEXT NOT NULL,
    package_version TEXT NOT NULL,
    cve_id        TEXT NOT NULL,
    cvss_score    REAL,
    cvss_vector   TEXT,
    severity      TEXT NOT NULL,
    summary       TEXT,
    fixed_in      TEXT,
    references_json TEXT,
    fetched_at    BIGINT NOT NULL,
    expires_at    BIGINT NOT NULL
);
CREATE INDEX idx_cve_cache_lookup ON cve_cache(ecosystem, package_name, package_version);
CREATE INDEX idx_cve_cache_cve_id ON cve_cache(cve_id);
CREATE INDEX idx_cve_cache_expires_at ON cve_cache(expires_at);
CREATE UNIQUE INDEX uq_cve_cache_entry ON cve_cache(ecosystem, package_name, package_version, cve_id);

CREATE TABLE container_cves (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL,
    cve_id        TEXT NOT NULL,
    severity      TEXT NOT NULL,
    cvss_score    REAL,
    summary       TEXT,
    fixed_in      TEXT,
    first_detected_at BIGINT NOT NULL,
    resolved_at   BIGINT
);
CREATE INDEX idx_container_cves_container_id ON container_cves(container_id);
CREATE INDEX idx_container_cves_severity ON container_cves(severity);
CREATE UNIQUE INDEX uq_container_cves ON container_cves(container_id, cve_id);

CREATE TABLE version_pins (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL,
    image         TEXT NOT NULL,
    pinned_tag    TEXT NOT NULL,
    pinned_digest TEXT NOT NULL,
    reason        TEXT,
    pinned_at     BIGINT NOT NULL
);
CREATE UNIQUE INDEX uq_version_pins_container ON version_pins(container_id);

CREATE TABLE update_exclusions (
    id            TEXT PRIMARY KEY NOT NULL,
    pattern       TEXT NOT NULL,
    pattern_type  TEXT NOT NULL,
    created_at    BIGINT NOT NULL
);
CREATE UNIQUE INDEX uq_update_exclusions_pattern ON update_exclusions(pattern, pattern_type);

CREATE TABLE risk_score_history (
    id            TEXT PRIMARY KEY NOT NULL,
    container_id  TEXT NOT NULL,
    score         INTEGER NOT NULL,
    factors_json  TEXT NOT NULL,
    recorded_at   BIGINT NOT NULL
);
CREATE INDEX idx_risk_score_history_container_time ON risk_score_history(container_id, recorded_at);
CREATE INDEX idx_risk_score_history_recorded_at ON risk_score_history(recorded_at);

CREATE TABLE risk_acknowledgments (
    id            TEXT PRIMARY KEY NOT NULL,
    container_external_id TEXT NOT NULL,
    finding_type  TEXT NOT NULL,
    finding_key   TEXT NOT NULL DEFAULT '',
    acknowledged_by TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    acknowledged_at BIGINT NOT NULL,
    UNIQUE(container_external_id, finding_type, finding_key)
);
CREATE INDEX idx_risk_ack_container ON risk_acknowledgments(container_external_id);

CREATE TABLE digest_baselines (
    container_id  TEXT PRIMARY KEY NOT NULL,
    image         TEXT NOT NULL,
    tag           TEXT NOT NULL,
    remote_digest TEXT NOT NULL,
    checked_at    BIGINT NOT NULL DEFAULT 0
);

-- ============================================================== swarm ========
CREATE TABLE swarm_nodes (
    id            TEXT PRIMARY KEY NOT NULL,                -- uid.SwarmNode(agent, node_id)
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    node_id       TEXT NOT NULL,
    hostname      TEXT NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('manager','worker')),
    status        TEXT NOT NULL CHECK(status IN ('ready','down','disconnected','unknown')),
    availability  TEXT NOT NULL CHECK(availability IN ('active','pause','drain')),
    engine_version TEXT NOT NULL DEFAULT '',
    address       TEXT NOT NULL DEFAULT '',
    task_count    INTEGER NOT NULL DEFAULT 0,
    first_seen_at BIGINT NOT NULL,
    last_seen_at  BIGINT NOT NULL,
    last_status_change_at BIGINT NOT NULL,
    UNIQUE(agent_id, node_id)
);
CREATE INDEX idx_swarm_nodes_status ON swarm_nodes(status);

-- swarm_services / swarm_tasks: per-agent swarm topology. IF NOT EXISTS because
-- the 22_swarm_topology forward migration may already have created them before
-- this canonical schema runs during the one-time UUID conversion.
CREATE TABLE IF NOT EXISTS swarm_services (
    id               TEXT PRIMARY KEY NOT NULL,                 -- uid.SwarmService(agent, service_id)
    agent_id         TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    service_id       TEXT NOT NULL,
    name             TEXT NOT NULL,
    image            TEXT NOT NULL DEFAULT '',
    mode             TEXT NOT NULL DEFAULT '',
    desired_replicas INTEGER NOT NULL DEFAULT 0,
    running_replicas INTEGER NOT NULL DEFAULT 0,
    labels           TEXT NOT NULL DEFAULT '{}',
    stack_name       TEXT NOT NULL DEFAULT '',
    created_at       BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, service_id)
);
CREATE INDEX IF NOT EXISTS idx_swarm_services_agent ON swarm_services(agent_id);
CREATE INDEX IF NOT EXISTS idx_swarm_services_stack ON swarm_services(agent_id, stack_name);

CREATE TABLE IF NOT EXISTS swarm_tasks (
    id            TEXT PRIMARY KEY NOT NULL,                    -- uid.SwarmTask(agent, task_id)
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    task_id       TEXT NOT NULL,
    service_id    TEXT NOT NULL,
    node_id       TEXT NOT NULL DEFAULT '',
    slot          INTEGER NOT NULL DEFAULT 0,
    state         TEXT NOT NULL DEFAULT '',
    desired_state TEXT NOT NULL DEFAULT '',
    container_id  TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    exit_code     INTEGER,
    timestamp     BIGINT NOT NULL DEFAULT 0,
    node_hostname TEXT NOT NULL DEFAULT '',
    UNIQUE(agent_id, task_id)
);
CREATE INDEX IF NOT EXISTS idx_swarm_tasks_agent ON swarm_tasks(agent_id);
CREATE INDEX IF NOT EXISTS idx_swarm_tasks_service ON swarm_tasks(agent_id, service_id);

-- ============================================================== kubernetes ===
-- Per-agent Kubernetes topology. IF NOT EXISTS because the 23_kubernetes_topology
-- forward migration may already have created these before this canonical schema
-- runs during the one-time UUID conversion.
CREATE TABLE IF NOT EXISTS kubernetes_namespaces (
    id        TEXT PRIMARY KEY NOT NULL,                       -- uid.Namespace(agent, name)
    agent_id  TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    UNIQUE(agent_id, name)
);
CREATE INDEX IF NOT EXISTS idx_k8s_namespaces_agent ON kubernetes_namespaces(agent_id);

CREATE TABLE IF NOT EXISTS kubernetes_workloads (
    id               TEXT PRIMARY KEY NOT NULL,                -- uid.K8sWorkload(agent, workload_id)
    agent_id         TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    workload_id      TEXT NOT NULL,                            -- "{namespace}/{kind}/{name}"
    name             TEXT NOT NULL,
    namespace        TEXT NOT NULL,
    kind             TEXT NOT NULL,
    images           TEXT NOT NULL DEFAULT '[]',
    ready_replicas   INTEGER NOT NULL DEFAULT 0,
    desired_replicas INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT '',
    created_at       BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, workload_id)
);
CREATE INDEX IF NOT EXISTS idx_k8s_workloads_agent ON kubernetes_workloads(agent_id);
CREATE INDEX IF NOT EXISTS idx_k8s_workloads_ns ON kubernetes_workloads(agent_id, namespace);

CREATE TABLE IF NOT EXISTS kubernetes_pods (
    id            TEXT PRIMARY KEY NOT NULL,                   -- uid.Pod(agent, namespace, name)
    agent_id      TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    namespace     TEXT NOT NULL,
    name          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT '',
    status_reason TEXT NOT NULL DEFAULT '',
    restart_count INTEGER NOT NULL DEFAULT 0,
    node_name     TEXT NOT NULL DEFAULT '',
    pod_ip        TEXT NOT NULL DEFAULT '',
    host_ip       TEXT NOT NULL DEFAULT '',
    workload_ref  TEXT NOT NULL DEFAULT '',
    containers    TEXT NOT NULL DEFAULT '[]',
    created_at    BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, namespace, name)
);
CREATE INDEX IF NOT EXISTS idx_k8s_pods_agent ON kubernetes_pods(agent_id);
CREATE INDEX IF NOT EXISTS idx_k8s_pods_ns ON kubernetes_pods(agent_id, namespace);
CREATE INDEX IF NOT EXISTS idx_k8s_pods_workload ON kubernetes_pods(agent_id, workload_ref);

CREATE TABLE IF NOT EXISTS kubernetes_nodes (
    id                         TEXT PRIMARY KEY NOT NULL,      -- uid.K8sNode(agent, name)
    agent_id                   TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    name                       TEXT NOT NULL,
    roles                      TEXT NOT NULL DEFAULT '[]',
    status                     TEXT NOT NULL DEFAULT '',
    running_pods               INTEGER NOT NULL DEFAULT 0,
    kubernetes_version         TEXT NOT NULL DEFAULT '',
    os_image                   TEXT NOT NULL DEFAULT '',
    architecture               TEXT NOT NULL DEFAULT '',
    capacity_cpu_millicores    BIGINT NOT NULL DEFAULT 0,
    capacity_memory_bytes      BIGINT NOT NULL DEFAULT 0,
    capacity_pods              BIGINT NOT NULL DEFAULT 0,
    allocatable_cpu_millicores BIGINT NOT NULL DEFAULT 0,
    allocatable_memory_bytes   BIGINT NOT NULL DEFAULT 0,
    allocatable_pods           BIGINT NOT NULL DEFAULT 0,
    created_at                 BIGINT NOT NULL DEFAULT 0,
    UNIQUE(agent_id, name)
);
CREATE INDEX IF NOT EXISTS idx_k8s_nodes_agent ON kubernetes_nodes(agent_id);

CREATE TABLE IF NOT EXISTS kubernetes_events (
    id                 TEXT PRIMARY KEY NOT NULL,
    agent_id           TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000' REFERENCES agents(id) ON DELETE CASCADE,
    involved_kind      TEXT NOT NULL DEFAULT '',
    involved_namespace TEXT NOT NULL DEFAULT '',
    involved_name      TEXT NOT NULL DEFAULT '',
    type               TEXT NOT NULL DEFAULT '',
    reason             TEXT NOT NULL DEFAULT '',
    message            TEXT NOT NULL DEFAULT '',
    source             TEXT NOT NULL DEFAULT '',
    first_seen         BIGINT NOT NULL DEFAULT 0,
    last_seen          BIGINT NOT NULL DEFAULT 0,
    count              INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_k8s_events_object ON kubernetes_events(agent_id, involved_kind, involved_namespace, involved_name);

-- ============================================================== mcp oauth ====
CREATE TABLE mcp_oauth_codes (
    code_hash     TEXT PRIMARY KEY NOT NULL,
    client_id     TEXT NOT NULL,
    redirect_uri  TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    code_challenge_method TEXT NOT NULL DEFAULT 'S256',
    scope         TEXT DEFAULT '',
    expires_at    BIGINT NOT NULL,
    used          INTEGER NOT NULL DEFAULT 0,
    created_at    BIGINT NOT NULL
);

CREATE TABLE mcp_oauth_tokens (
    token_hash    TEXT PRIMARY KEY NOT NULL,
    token_type    TEXT NOT NULL CHECK(token_type IN ('access','refresh')),
    client_id     TEXT NOT NULL,
    scope         TEXT DEFAULT '',
    expires_at    BIGINT NOT NULL,
    revoked       INTEGER NOT NULL DEFAULT 0,
    family_id     TEXT NOT NULL,
    created_at    BIGINT NOT NULL
);
CREATE INDEX idx_mcp_oauth_tokens_family_id ON mcp_oauth_tokens(family_id);
CREATE INDEX idx_mcp_oauth_tokens_expires_at ON mcp_oauth_tokens(expires_at);

-- ============================================================== webhooks =====
CREATE TABLE webhook_subscriptions (
    id            TEXT PRIMARY KEY NOT NULL,
    name          TEXT NOT NULL,
    url           TEXT NOT NULL,
    secret        TEXT,
    event_types   TEXT NOT NULL DEFAULT '["*"]',
    is_active     INTEGER NOT NULL DEFAULT 1,
    last_delivery_status TEXT,
    last_delivery_at BIGINT,
    failure_count INTEGER NOT NULL DEFAULT 0,
    created_at    BIGINT NOT NULL DEFAULT 0
);

-- =========================================================== schema meta =====
CREATE TABLE schema_meta (
    key           TEXT PRIMARY KEY NOT NULL,
    value         TEXT NOT NULL
);
INSERT INTO schema_meta (key, value) VALUES ('format', 'uuid-v1');
