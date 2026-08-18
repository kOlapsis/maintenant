// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/kolapsis/maintenant/internal/uid"
)

// uuidSchemaSQL is the UUID-native, portable target schema. It is applied once,
// in place, by convertToUUID — replacing the integer-keyed schema built by
// migrations 1-21.
//
//go:embed uuid_schema.sql
var uuidSchemaSQL string

// legacyTables are the integer-keyed tables produced by migrations 1-21. They
// are renamed aside, copied into the new schema, then dropped.
var legacyTables = []string{
	"containers", "state_transitions", "resource_snapshots", "resource_alert_configs",
	"resource_hourly", "resource_daily", "endpoints", "check_results",
	"cert_monitors", "cert_check_results", "cert_chain_entries",
	"heartbeats", "heartbeat_pings", "heartbeat_executions",
	"alerts", "notification_channels", "notification_deliveries", "silence_rules",
	"alert_triggers", "alert_trigger_channels",
	"escalation_policies", "escalation_runs", "escalation_deliveries",
	"status_components", "status_component_monitors",
	"incidents", "incident_components", "incident_updates",
	"maintenance_windows", "maintenance_components",
	"status_subscribers", "status_page_settings", "status_page_assets",
	"status_page_footer_links", "status_page_faq_items",
	"image_update_scans", "image_updates", "cve_cache", "container_cves",
	"version_pins", "update_exclusions", "risk_score_history", "risk_acknowledgments",
	"digest_baselines", "swarm_nodes", "mcp_oauth_codes", "mcp_oauth_tokens",
	"webhook_subscriptions",
}

// convertToUUID rebuilds the integer-keyed schema (migrations 1-21) into the
// UUID-native schema, in place and without data loss. It is a one-time forward
// migration: once the schema_meta marker exists the conversion has run and is
// not repeated.
//
// All work happens on a single dedicated connection with foreign keys disabled
// (re-enabled and validated at the end) and inside one transaction, so a failure
// rolls back to the original schema.
func convertToUUID(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	converted, err := hasTable(ctx, db, "schema_meta")
	if err != nil {
		return fmt.Errorf("uuid convert: probe schema_meta: %w", err)
	}
	if converted {
		return nil
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("uuid convert: acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Register UUID minting/derivation as SQL functions so the copy can run as
	// set-based INSERT ... SELECT statements (deriving natural-key ids via joins).
	if err := registerUIDFuncs(ctx, conn); err != nil {
		return fmt.Errorf("uuid convert: register funcs: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("uuid convert: fk off: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA legacy_alter_table=ON"); err != nil {
		return fmt.Errorf("uuid convert: legacy_alter_table: %w", err)
	}

	logger.Info("uuid conversion starting")

	if err := runConversion(ctx, conn); err != nil {
		return err
	}

	if err := foreignKeyCheck(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA legacy_alter_table=OFF"); err != nil {
		return fmt.Errorf("uuid convert: legacy_alter_table reset: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("uuid convert: fk on: %w", err)
	}

	logger.Info("uuid conversion complete")
	return nil
}

// runConversion performs the DDL + data copy inside one transaction.
func runConversion(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("uuid convert: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	exec := func(desc, query string) error {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("uuid convert: %s: %w", desc, err)
		}
		return nil
	}

	// Drop legacy named indexes (they would collide with the new schema's index
	// names) and rename the legacy tables aside.
	idxNames, err := legacyIndexNames(ctx, tx)
	if err != nil {
		return err
	}
	for _, idx := range idxNames {
		if err := exec("drop index "+idx, fmt.Sprintf("DROP INDEX %q", idx)); err != nil {
			return err
		}
	}
	for _, t := range legacyTables {
		if err := exec("rename "+t, fmt.Sprintf("ALTER TABLE %q RENAME TO %q", t, "_old_"+t)); err != nil {
			return err
		}
	}

	// Drop any pre-existing agents/enrollment_tokens from the never-deployed
	// agent migrations (22-23) so the new schema can recreate them cleanly. No-op
	// on production databases (migrations 1-21 never created these tables).
	for _, t := range []string{"agents", "enrollment_tokens"} {
		if err := exec("drop unreleased "+t, fmt.Sprintf("DROP TABLE IF EXISTS %q", t)); err != nil {
			return err
		}
	}

	// Create the UUID-native schema (final table + index names, sentinel agent,
	// schema_meta marker).
	if err := exec("create schema", uuidSchemaSQL); err != nil {
		return err
	}

	// Copy data. Statements run in dependency order; natural-key parents are
	// re-derived in child joins, minted-and-referenced parents use temp maps.
	for _, st := range copyStatements() {
		if err := exec(st.desc, st.sql); err != nil {
			return err
		}
	}

	for _, t := range legacyTables {
		if err := exec("drop "+t, fmt.Sprintf("DROP TABLE %q", "_old_"+t)); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("uuid convert: commit: %w", err)
	}
	committed = true
	return nil
}

// registerUIDFuncs exposes the uid package as SQL functions on conn.
func registerUIDFuncs(ctx context.Context, conn *sql.Conn) error {
	return conn.Raw(func(driverConn any) error {
		c, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("not a sqlite3 conn: %T", driverConn)
		}
		if err := c.RegisterFunc("mnt_uuid7", func() string { return uid.New() }, false); err != nil {
			return err
		}
		if err := c.RegisterFunc("mnt_container_id", uid.Container, true); err != nil {
			return err
		}
		if err := c.RegisterFunc("mnt_endpoint_id", uid.EndpointLabel, true); err != nil {
			return err
		}
		if err := c.RegisterFunc("mnt_cert_id", func(agent, host string, port int64, serverName string) string {
			return uid.CertMonitor(agent, host, int(port), serverName)
		}, true); err != nil {
			return err
		}
		if err := c.RegisterFunc("mnt_swarm_id", uid.SwarmNode, true); err != nil {
			return err
		}
		return nil
	})
}

type stmt struct{ desc, sql string }

// s is the local sentinel agent id, injected into the copy SQL.
const s = uid.LocalAgent

// epoch returns a SQL expression coercing a legacy timestamp column to an
// epoch-second integer regardless of how it was stored. The app wrote some
// columns as epoch integers (time.Unix) and others as datetime strings (or left
// them to DEFAULT CURRENT_TIMESTAMP). Integers/reals pass through; strings go
// through SQLite's date parser; NULL stays NULL.
func epoch(col string) string {
	return "CAST(CASE WHEN typeof(" + col + ") IN ('integer','real') THEN " + col +
		" ELSE strftime('%s', " + col + ") END AS INTEGER)"
}

// copyStatements returns the ordered INSERT ... SELECT statements that move data
// from the renamed _old_* tables into the UUID-native tables.
func copyStatements() []stmt {
	return []stmt{
		// -------- temp maps for minted-and-referenced entities (random ids) -----
		{"map alerts", `CREATE TEMP TABLE _map_alert AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_alerts`},
		{"map channels", `CREATE TEMP TABLE _map_channel AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_notification_channels`},
		{"map policies", `CREATE TEMP TABLE _map_policy AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_escalation_policies`},
		{"map runs", `CREATE TEMP TABLE _map_run AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_escalation_runs`},
		{"map triggers", `CREATE TEMP TABLE _map_trigger AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_alert_triggers`},
		{"map incidents", `CREATE TEMP TABLE _map_incident AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_incidents`},
		{"map maintenance", `CREATE TEMP TABLE _map_maint AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_maintenance_windows`},
		{"map components", `CREATE TEMP TABLE _map_component AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_status_components`},
		{"map scans", `CREATE TEMP TABLE _map_scan AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_image_update_scans`},
		{"map cert_checks", `CREATE TEMP TABLE _map_certcheck AS SELECT id AS old_id, mnt_uuid7() AS new_id FROM _old_cert_check_results`},

		// -------- containers + children (natural-key derived) -------------------
		{"containers", `INSERT INTO containers
			(id, agent_id, external_id, name, image, state, health_status, has_health_check,
			 orchestration_group, orchestration_unit, custom_group, is_ignored, alert_severity,
			 restart_threshold, alert_channels, archived, first_seen_at, last_state_change_at, archived_at,
			 runtime_type, error_detail, controller_kind, namespace, pod_count, ready_count,
			 compose_working_dir, swarm_service_id, swarm_service_name, swarm_service_mode,
			 swarm_node_id, swarm_task_slot, swarm_desired_replicas)
			SELECT mnt_container_id('` + s + `', external_id), '` + s + `', external_id, name, image, state,
			 health_status, has_health_check, orchestration_group, orchestration_unit, custom_group,
			 is_ignored, alert_severity, restart_threshold, alert_channels, archived, first_seen_at,
			 last_state_change_at, archived_at, runtime_type, error_detail, controller_kind, namespace,
			 pod_count, ready_count, compose_working_dir, swarm_service_id, swarm_service_name,
			 swarm_service_mode, swarm_node_id, swarm_task_slot, swarm_desired_replicas
			FROM _old_containers`},

		{"state_transitions", `INSERT INTO state_transitions
			(id, container_id, previous_state, new_state, previous_health, new_health, exit_code, log_snippet, timestamp)
			SELECT mnt_uuid7(), mnt_container_id('` + s + `', c.external_id), st.previous_state, st.new_state,
			 st.previous_health, st.new_health, st.exit_code, st.log_snippet, st.timestamp
			FROM _old_state_transitions st JOIN _old_containers c ON st.container_id = c.id`},

		{"resource_snapshots", `INSERT INTO resource_snapshots
			(id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes,
			 block_read_bytes, block_write_bytes, timestamp)
			SELECT mnt_uuid7(), mnt_container_id('` + s + `', c.external_id), '` + s + `', rs.cpu_percent,
			 rs.mem_used, rs.mem_limit, rs.net_rx_bytes, rs.net_tx_bytes, rs.block_read_bytes,
			 rs.block_write_bytes, rs.timestamp
			FROM _old_resource_snapshots rs JOIN _old_containers c ON rs.container_id = c.id`},

		{"resource_alert_configs", `INSERT INTO resource_alert_configs
			(id, container_id, cpu_threshold, mem_threshold, enabled, alert_state,
			 cpu_consecutive_breaches, mem_consecutive_breaches, last_alerted_at, created_at, updated_at)
			SELECT mnt_uuid7(), mnt_container_id('` + s + `', c.external_id), rac.cpu_threshold, rac.mem_threshold,
			 rac.enabled, rac.alert_state, rac.cpu_consecutive_breaches, rac.mem_consecutive_breaches,
			 rac.last_alerted_at, rac.created_at, rac.updated_at
			FROM _old_resource_alert_configs rac JOIN _old_containers c ON rac.container_id = c.id`},

		// resource_hourly/daily reference containers by the integer id; re-derive
		// via join so they point at the container UUID.
		{"resource_hourly", `INSERT INTO resource_hourly
			(id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes,
			 avg_block_read_bytes, avg_block_write_bytes, sample_count)
			SELECT mnt_uuid7(), mnt_container_id('` + s + `', c.external_id), rh.bucket, rh.avg_cpu_percent,
			 rh.avg_mem_used, rh.avg_mem_limit, rh.avg_net_rx_bytes, rh.avg_net_tx_bytes,
			 rh.avg_block_read_bytes, rh.avg_block_write_bytes, rh.sample_count
			FROM _old_resource_hourly rh JOIN _old_containers c ON rh.container_id = c.id`},
		{"resource_daily", `INSERT INTO resource_daily
			(id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
			SELECT mnt_uuid7(), mnt_container_id('` + s + `', c.external_id), rd.bucket, rd.avg_cpu_percent,
			 rd.avg_mem_used, rd.avg_mem_limit, rd.avg_net_rx_bytes, rd.avg_net_tx_bytes, rd.sample_count
			FROM _old_resource_daily rd JOIN _old_containers c ON rd.container_id = c.id`},

		// -------- endpoints + checks (natural-key derived) ----------------------
		{"endpoints", `INSERT INTO endpoints
			(id, agent_id, container_name, label_key, external_id, endpoint_type, target, status, alert_state,
			 consecutive_failures, consecutive_successes, last_check_at, last_response_time_ms, last_http_status,
			 last_error, config_json, active, first_seen_at, last_seen_at, source, name)
			SELECT mnt_endpoint_id('` + s + `', container_name, label_key), '` + s + `', container_name, label_key,
			 external_id, endpoint_type, target, status, alert_state, consecutive_failures, consecutive_successes,
			 last_check_at, last_response_time_ms, last_http_status, last_error, config_json, active,
			 first_seen_at, last_seen_at, source, name
			FROM _old_endpoints`},

		{"check_results", `INSERT INTO check_results
			(id, endpoint_id, success, response_time_ms, http_status, error_message, timestamp)
			SELECT mnt_uuid7(), mnt_endpoint_id('` + s + `', e.container_name, e.label_key), cr.success,
			 cr.response_time_ms, cr.http_status, cr.error_message, cr.timestamp
			FROM _old_check_results cr JOIN _old_endpoints e ON cr.endpoint_id = e.id`},

		// -------- cert monitors + checks (natural-key derived) ------------------
		// endpoint_id (nullable) is re-derived from the linked endpoint.
		{"cert_monitors", `INSERT INTO cert_monitors
			(id, agent_id, hostname, port, source, endpoint_id, status, check_interval_seconds,
			 warning_thresholds_json, last_alerted_threshold, last_check_at, next_check_at, last_error,
			 created_at, external_id, server_name)
			SELECT mnt_cert_id('` + s + `', cm.hostname, cm.port, cm.server_name), '` + s + `', cm.hostname, cm.port, cm.source,
			 (SELECT mnt_endpoint_id('` + s + `', e.container_name, e.label_key) FROM _old_endpoints e WHERE e.id = cm.endpoint_id),
			 cm.status, cm.check_interval_seconds, cm.warning_thresholds_json, cm.last_alerted_threshold,
			 cm.last_check_at, cm.next_check_at, cm.last_error, cm.created_at, cm.external_id, cm.server_name
			FROM _old_cert_monitors cm`},

		{"cert_check_results", `INSERT INTO cert_check_results
			(id, monitor_id, subject_cn, issuer_cn, issuer_org, sans_json, serial_number, signature_algorithm,
			 not_before, not_after, chain_valid, chain_error, hostname_match, error_message, checked_at,
			 ocsp_stapled, ocsp_status, ocsp_produced_at, ocsp_next_update, ocsp_error)
			SELECT mcc.new_id, mnt_cert_id('` + s + `', cm.hostname, cm.port, cm.server_name), ccr.subject_cn, ccr.issuer_cn,
			 ccr.issuer_org, ccr.sans_json, ccr.serial_number, ccr.signature_algorithm, ccr.not_before,
			 ccr.not_after, ccr.chain_valid, ccr.chain_error, ccr.hostname_match, ccr.error_message,
			 ccr.checked_at, ccr.ocsp_stapled, ccr.ocsp_status, ccr.ocsp_produced_at, ccr.ocsp_next_update, ccr.ocsp_error
			FROM _old_cert_check_results ccr JOIN _old_cert_monitors cm ON ccr.monitor_id = cm.id
			JOIN _map_certcheck mcc ON ccr.id = mcc.old_id`},

		// cert_chain_entries -> cert_check_results via the same temp map.
		{"cert_chain_entries", `INSERT INTO cert_chain_entries
			(id, check_result_id, position, subject_cn, issuer_cn, not_before, not_after)
			SELECT mnt_uuid7(), m.new_id, cce.position, cce.subject_cn, cce.issuer_cn, cce.not_before, cce.not_after
			FROM _old_cert_chain_entries cce JOIN _map_certcheck m ON cce.check_result_id = m.old_id`},

		// -------- heartbeats + children (id = ping token) -----------------------
		{"heartbeats", `INSERT INTO heartbeats
			(id, agent_id, name, status, alert_state, interval_seconds, grace_seconds, last_ping_at,
			 next_deadline_at, current_run_started_at, last_exit_code, last_duration_ms,
			 consecutive_failures, consecutive_successes, active, created_at, updated_at)
			SELECT uuid, '` + s + `', name, status, alert_state, interval_seconds, grace_seconds, last_ping_at,
			 next_deadline_at, current_run_started_at, last_exit_code, last_duration_ms, consecutive_failures,
			 consecutive_successes, active, created_at, updated_at
			FROM _old_heartbeats`},

		{"heartbeat_pings", `INSERT INTO heartbeat_pings
			(id, heartbeat_id, ping_type, exit_code, source_ip, http_method, payload, timestamp)
			SELECT mnt_uuid7(), h.uuid, p.ping_type, p.exit_code, p.source_ip, p.http_method, p.payload, p.timestamp
			FROM _old_heartbeat_pings p JOIN _old_heartbeats h ON p.heartbeat_id = h.id`},

		{"heartbeat_executions", `INSERT INTO heartbeat_executions
			(id, heartbeat_id, started_at, completed_at, duration_ms, exit_code, outcome, payload)
			SELECT mnt_uuid7(), h.uuid, e.started_at, e.completed_at, e.duration_ms, e.exit_code, e.outcome, e.payload
			FROM _old_heartbeat_executions e JOIN _old_heartbeats h ON e.heartbeat_id = h.id`},

		// -------- swarm nodes (natural-key derived) -----------------------------
		{"swarm_nodes", `INSERT INTO swarm_nodes
			(id, agent_id, node_id, hostname, role, status, availability, engine_version, address,
			 task_count, first_seen_at, last_seen_at, last_status_change_at)
			SELECT mnt_swarm_id('` + s + `', node_id), '` + s + `', node_id, hostname, role, status, availability,
			 engine_version, address, task_count, first_seen_at, last_seen_at, last_status_change_at
			FROM _old_swarm_nodes`},

		// -------- alerts (minted) + dependents ----------------------------------
		// entity_id is polymorphic (by entity_type) onto natural-key entities,
		// re-derived per type. resolved_by_id maps onto another alert.
		{"alerts", `INSERT INTO alerts
			(id, source, alert_type, severity, status, message, entity_type, entity_id, entity_name, details,
			 resolved_by_id, fired_at, resolved_at, created_at, acknowledged_at, acknowledged_by, escalated_at)
			SELECT ma.new_id, a.source, a.alert_type, a.severity, a.status, a.message, a.entity_type,
			 COALESCE(CASE a.entity_type
			   WHEN 'container'   THEN (SELECT mnt_container_id('` + s + `', c.external_id) FROM _old_containers c WHERE c.id = a.entity_id)
			   WHEN 'endpoint'    THEN (SELECT mnt_endpoint_id('` + s + `', e.container_name, e.label_key) FROM _old_endpoints e WHERE e.id = a.entity_id)
			   WHEN 'certificate' THEN (SELECT mnt_cert_id('` + s + `', cm.hostname, cm.port, cm.server_name) FROM _old_cert_monitors cm WHERE cm.id = a.entity_id)
			   WHEN 'heartbeat'   THEN (SELECT hb.uuid FROM _old_heartbeats hb WHERE hb.id = a.entity_id)
			   ELSE CAST(a.entity_id AS TEXT)
			 END, CAST(a.entity_id AS TEXT)),
			 a.entity_name, a.details,
			 (SELECT new_id FROM _map_alert WHERE old_id = a.resolved_by_id),
			 ` + epoch("a.fired_at") + `,
			 ` + epoch("a.resolved_at") + `,
			 ` + epoch("a.created_at") + `,
			 ` + epoch("a.acknowledged_at") + `, a.acknowledged_by,
			 ` + epoch("a.escalated_at") + `
			FROM _old_alerts a JOIN _map_alert ma ON a.id = ma.old_id`},

		// -------- notification channels (minted) + deliveries -------------------
		{"notification_channels", `INSERT INTO notification_channels
			(id, name, type, url, headers, enabled, created_at, updated_at)
			SELECT mc.new_id, nc.name, nc.type, nc.url, nc.headers, nc.enabled,
			 ` + epoch("nc.created_at") + `, ` + epoch("nc.updated_at") + `
			FROM _old_notification_channels nc JOIN _map_channel mc ON nc.id = mc.old_id`},

		{"notification_deliveries", `INSERT INTO notification_deliveries
			(id, alert_id, channel_id, status, attempts, last_error, created_at, updated_at)
			SELECT mnt_uuid7(), ma.new_id, mc.new_id, nd.status, nd.attempts, nd.last_error,
			 ` + epoch("nd.created_at") + `, ` + epoch("nd.updated_at") + `
			FROM _old_notification_deliveries nd
			JOIN _map_alert ma ON nd.alert_id = ma.old_id
			JOIN _map_channel mc ON nd.channel_id = mc.old_id`},

		// -------- silence rules (minted) ----------------------------------------
		{"silence_rules", `INSERT INTO silence_rules
			(id, entity_type, entity_id, source, reason, starts_at, duration_seconds, cancelled_at, created_at)
			SELECT mnt_uuid7(), sr.entity_type,
			 COALESCE(CASE sr.entity_type
			   WHEN 'container'   THEN (SELECT mnt_container_id('` + s + `', c.external_id) FROM _old_containers c WHERE c.id = sr.entity_id)
			   WHEN 'endpoint'    THEN (SELECT mnt_endpoint_id('` + s + `', e.container_name, e.label_key) FROM _old_endpoints e WHERE e.id = sr.entity_id)
			   WHEN 'certificate' THEN (SELECT mnt_cert_id('` + s + `', cm.hostname, cm.port, cm.server_name) FROM _old_cert_monitors cm WHERE cm.id = sr.entity_id)
			   WHEN 'heartbeat'   THEN (SELECT hb.uuid FROM _old_heartbeats hb WHERE hb.id = sr.entity_id)
			   WHEN NULL THEN NULL
			   ELSE CASE WHEN sr.entity_id IS NULL THEN NULL ELSE CAST(sr.entity_id AS TEXT) END
			 END, CASE WHEN sr.entity_id IS NULL THEN NULL ELSE CAST(sr.entity_id AS TEXT) END),
			 sr.source, sr.reason, ` + epoch("sr.starts_at") + `, sr.duration_seconds,
			 ` + epoch("sr.cancelled_at") + `, ` + epoch("sr.created_at") + `
			FROM _old_silence_rules sr`},

		// -------- alert triggers (minted) + channels join -----------------------
		{"alert_triggers", `INSERT INTO alert_triggers
			(id, name, filter_severities, filter_sources, filter_scopes, filter_tags, enabled, notify_on_resolve, created_at, updated_at)
			SELECT mt.new_id, t.name, t.filter_severities, t.filter_sources, t.filter_scopes, t.filter_tags,
			 t.enabled, t.notify_on_resolve, ` + epoch("t.created_at") + `, ` + epoch("t.updated_at") + `
			FROM _old_alert_triggers t JOIN _map_trigger mt ON t.id = mt.old_id`},

		{"alert_trigger_channels", `INSERT INTO alert_trigger_channels (trigger_id, channel_id)
			SELECT mt.new_id, mc.new_id FROM _old_alert_trigger_channels atc
			JOIN _map_trigger mt ON atc.trigger_id = mt.old_id
			JOIN _map_channel mc ON atc.channel_id = mc.old_id`},

		// -------- escalation policies/runs/deliveries (minted) ------------------
		{"escalation_policies", `INSERT INTO escalation_policies
			(id, name, active, active_before_downgrade, severities_json, scopes_json, tags_json, levels_json,
			 created_at, created_by, updated_at, updated_by)
			SELECT mp.new_id, p.name, p.active, p.active_before_downgrade, p.severities_json, p.scopes_json,
			 p.tags_json, p.levels_json, ` + epoch("p.created_at") + `, p.created_by,
			 ` + epoch("p.updated_at") + `, p.updated_by
			FROM _old_escalation_policies p JOIN _map_policy mp ON p.id = mp.old_id`},

		{"escalation_runs", `INSERT INTO escalation_runs
			(id, policy_id, policy_snapshot_json, alert_id, status, last_executed_level_index, started_at, ended_at, next_action_at)
			SELECT mr.new_id, (SELECT new_id FROM _map_policy WHERE old_id = r.policy_id), r.policy_snapshot_json,
			 ma.new_id, r.status, r.last_executed_level_index, ` + epoch("r.started_at") + `,
			 ` + epoch("r.ended_at") + `, ` + epoch("r.next_action_at") + `
			FROM _old_escalation_runs r JOIN _map_run mr ON r.id = mr.old_id
			JOIN _map_alert ma ON r.alert_id = ma.old_id`},

		{"escalation_deliveries", `INSERT INTO escalation_deliveries
			(id, run_id, level_index, channel_id, status, error, attempt_started_at, sent_at)
			SELECT mnt_uuid7(), mr.new_id, d.level_index, (SELECT new_id FROM _map_channel WHERE old_id = d.channel_id),
			 d.status, d.error, ` + epoch("d.attempt_started_at") + `, ` + epoch("d.sent_at") + `
			FROM _old_escalation_deliveries d JOIN _map_run mr ON d.run_id = mr.old_id`},

		// -------- status components (minted) + monitors -------------------------
		{"status_components", `INSERT INTO status_components
			(id, composition_mode, match_all_type, display_name, display_order, visible, status_override,
			 auto_incident, created_at, updated_at)
			SELECT mco.new_id, sc.composition_mode, sc.match_all_type, sc.display_name, sc.display_order,
			 sc.visible, sc.status_override, sc.auto_incident, sc.created_at, sc.updated_at
			FROM _old_status_components sc JOIN _map_component mco ON sc.id = mco.old_id`},

		{"status_component_monitors", `INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id)
			SELECT mco.new_id, scm.monitor_type,
			 COALESCE(CASE scm.monitor_type
			   WHEN 'container'   THEN (SELECT mnt_container_id('` + s + `', c.external_id) FROM _old_containers c WHERE c.id = scm.monitor_id)
			   WHEN 'endpoint'    THEN (SELECT mnt_endpoint_id('` + s + `', e.container_name, e.label_key) FROM _old_endpoints e WHERE e.id = scm.monitor_id)
			   WHEN 'certificate' THEN (SELECT mnt_cert_id('` + s + `', cm.hostname, cm.port, cm.server_name) FROM _old_cert_monitors cm WHERE cm.id = scm.monitor_id)
			   WHEN 'heartbeat'   THEN (SELECT hb.uuid FROM _old_heartbeats hb WHERE hb.id = scm.monitor_id)
			   ELSE CAST(scm.monitor_id AS TEXT)
			 END, CAST(scm.monitor_id AS TEXT))
			FROM _old_status_component_monitors scm JOIN _map_component mco ON scm.component_id = mco.old_id`},

		// -------- incidents/maintenance (minted, cross-referencing) -------------
		{"incidents", `INSERT INTO incidents
			(id, title, severity, status, is_maintenance, maintenance_window_id, created_at, resolved_at, updated_at)
			SELECT mi.new_id, i.title, i.severity, i.status, i.is_maintenance,
			 (SELECT new_id FROM _map_maint WHERE old_id = i.maintenance_window_id),
			 i.created_at, i.resolved_at, i.updated_at
			FROM _old_incidents i JOIN _map_incident mi ON i.id = mi.old_id`},

		{"maintenance_windows", `INSERT INTO maintenance_windows
			(id, title, description, starts_at, ends_at, active, incident_id, created_at, updated_at)
			SELECT mm.new_id, w.title, w.description, w.starts_at, w.ends_at, w.active,
			 (SELECT new_id FROM _map_incident WHERE old_id = w.incident_id), w.created_at, w.updated_at
			FROM _old_maintenance_windows w JOIN _map_maint mm ON w.id = mm.old_id`},

		{"incident_components", `INSERT INTO incident_components (incident_id, component_id)
			SELECT mi.new_id, mco.new_id FROM _old_incident_components ic
			JOIN _map_incident mi ON ic.incident_id = mi.old_id
			JOIN _map_component mco ON ic.component_id = mco.old_id`},

		{"incident_updates", `INSERT INTO incident_updates
			(id, incident_id, status, message, is_auto, alert_id, created_at)
			SELECT mnt_uuid7(), mi.new_id, iu.status, iu.message, iu.is_auto,
			 (SELECT new_id FROM _map_alert WHERE old_id = iu.alert_id), iu.created_at
			FROM _old_incident_updates iu JOIN _map_incident mi ON iu.incident_id = mi.old_id`},

		{"maintenance_components", `INSERT INTO maintenance_components (maintenance_id, component_id)
			SELECT mm.new_id, mco.new_id FROM _old_maintenance_components mc
			JOIN _map_maint mm ON mc.maintenance_id = mm.old_id
			JOIN _map_component mco ON mc.component_id = mco.old_id`},

		// -------- status subscribers + page personalization (verbatim) ----------
		{"status_subscribers", `INSERT INTO status_subscribers
			(id, email, confirmed, confirm_token, confirm_expires, unsub_token, created_at)
			SELECT mnt_uuid7(), email, confirmed, confirm_token, confirm_expires, unsub_token, created_at
			FROM _old_status_subscribers`},
		{"status_page_settings", `INSERT INTO status_page_settings SELECT * FROM _old_status_page_settings`},
		{"status_page_assets", `INSERT INTO status_page_assets SELECT * FROM _old_status_page_assets`},
		{"status_page_footer_links", `INSERT INTO status_page_footer_links
			(id, position, label, url, created_at, updated_at)
			SELECT mnt_uuid7(), position, label, url, created_at, updated_at FROM _old_status_page_footer_links`},
		{"status_page_faq_items", `INSERT INTO status_page_faq_items
			(id, position, question, answer_md, answer_html, created_at, updated_at)
			SELECT mnt_uuid7(), position, question, answer_md, answer_html, created_at, updated_at FROM _old_status_page_faq_items`},

		// -------- update intelligence / CVE (container_id stays external_id) -----
		{"image_update_scans", `INSERT INTO image_update_scans
			(id, started_at, completed_at, containers_scanned, updates_found, errors, status)
			SELECT msc.new_id, sc.started_at, sc.completed_at, sc.containers_scanned, sc.updates_found, sc.errors, sc.status
			FROM _old_image_update_scans sc JOIN _map_scan msc ON sc.id = msc.old_id`},

		{"image_updates", `INSERT INTO image_updates
			(id, scan_id, container_id, container_name, image, current_tag, current_digest, registry, latest_tag,
			 latest_digest, update_type, published_at, changelog_url, changelog_summary, has_breaking_changes,
			 risk_score, previous_digest, source_url, status, detected_at)
			SELECT mnt_uuid7(), (SELECT new_id FROM _map_scan WHERE old_id = iu.scan_id), iu.container_id,
			 iu.container_name, iu.image, iu.current_tag, iu.current_digest, iu.registry, iu.latest_tag,
			 iu.latest_digest, iu.update_type, iu.published_at, iu.changelog_url, iu.changelog_summary,
			 iu.has_breaking_changes, iu.risk_score, iu.previous_digest, iu.source_url, iu.status, iu.detected_at
			FROM _old_image_updates iu`},

		{"cve_cache", `INSERT INTO cve_cache
			(id, ecosystem, package_name, package_version, cve_id, cvss_score, cvss_vector, severity, summary,
			 fixed_in, references_json, fetched_at, expires_at)
			SELECT mnt_uuid7(), ecosystem, package_name, package_version, cve_id, cvss_score, cvss_vector, severity,
			 summary, fixed_in, references_json, fetched_at, expires_at FROM _old_cve_cache`},

		{"container_cves", `INSERT INTO container_cves
			(id, container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at)
			SELECT mnt_uuid7(), container_id, cve_id, severity, cvss_score, summary, fixed_in, first_detected_at, resolved_at
			FROM _old_container_cves`},

		{"version_pins", `INSERT INTO version_pins
			(id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at)
			SELECT mnt_uuid7(), container_id, image, pinned_tag, pinned_digest, reason, pinned_at FROM _old_version_pins`},

		{"update_exclusions", `INSERT INTO update_exclusions (id, pattern, pattern_type, created_at)
			SELECT mnt_uuid7(), pattern, pattern_type, created_at FROM _old_update_exclusions`},

		{"risk_score_history", `INSERT INTO risk_score_history (id, container_id, score, factors_json, recorded_at)
			SELECT mnt_uuid7(), container_id, score, factors_json, recorded_at FROM _old_risk_score_history`},

		{"risk_acknowledgments", `INSERT INTO risk_acknowledgments
			(id, container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at)
			SELECT mnt_uuid7(), container_external_id, finding_type, finding_key, acknowledged_by, reason, acknowledged_at
			FROM _old_risk_acknowledgments`},

		{"digest_baselines", `INSERT INTO digest_baselines (container_id, image, tag, remote_digest, checked_at)
			SELECT container_id, image, tag, remote_digest, ` + epoch("checked_at") + ` FROM _old_digest_baselines`},

		// -------- mcp oauth + webhooks (natural / text keys) --------------------
		{"mcp_oauth_codes", `INSERT INTO mcp_oauth_codes SELECT * FROM _old_mcp_oauth_codes`},
		{"mcp_oauth_tokens", `INSERT INTO mcp_oauth_tokens SELECT * FROM _old_mcp_oauth_tokens`},
		{"webhook_subscriptions", `INSERT INTO webhook_subscriptions
			(id, name, url, secret, event_types, is_active, last_delivery_status, last_delivery_at, failure_count, created_at)
			SELECT id, name, url, secret, event_types, is_active, last_delivery_status,
			 ` + epoch("last_delivery_at") + `, failure_count, ` + epoch("created_at") + `
			FROM _old_webhook_subscriptions`},
	}
}

// --- helpers ---------------------------------------------------------------

func hasTable(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func legacyIndexNames(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, fmt.Errorf("uuid convert: list indexes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func foreignKeyCheck(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("uuid convert: foreign_key_check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var violations []string
	for rows.Next() {
		var table, fkRowid, parent, fkID sql.NullString
		if err := rows.Scan(&table, &fkRowid, &parent, &fkID); err != nil {
			return err
		}
		violations = append(violations, fmt.Sprintf("%s -> %s", table.String, parent.String))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(violations) > 0 {
		return fmt.Errorf("uuid convert: %d foreign key violations: %s",
			len(violations), strings.Join(violations, ", "))
	}
	return nil
}
