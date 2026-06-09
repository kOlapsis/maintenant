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
	"io"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

// setupLegacyDB opens a file-backed DB, applies migrations 1-21 (the pre-UUID
// schema), and returns it.
func setupLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	path := t.TempDir() + "/legacy.db"
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=ON")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	entries, err := fs.ReadDir(migrationFS, "migrations")
	require.NoError(t, err)
	type mig struct {
		n   int
		sql string
	}
	var ups []mig
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		require.NoError(t, err)
		data, err := fs.ReadFile(migrationFS, "migrations/"+name)
		require.NoError(t, err)
		ups = append(ups, mig{n, string(data)})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].n < ups[j].n })
	for _, m := range ups {
		_, err := db.Exec(m.sql)
		require.NoError(t, err, "apply migration %d", m.n)
	}
	return db
}

// seedLegacyGraph inserts a representative entity graph exercising the tricky
// transform paths: polymorphic refs, self-refs, cross-refs and DATETIME columns.
func seedLegacyGraph(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT INTO containers (external_id, name, image, state, first_seen_at, last_state_change_at) VALUES ('abc123','web','nginx','running',100,100)`,
		`INSERT INTO state_transitions (container_id, previous_state, new_state, timestamp) VALUES (1,'created','running',100)`,
		`INSERT INTO resource_snapshots (container_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp) VALUES (1,1.5,10,20,0,0,0,0,100)`,
		`INSERT INTO endpoints (container_name, label_key, external_id, endpoint_type, target, first_seen_at, last_seen_at) VALUES ('web','maintenant.endpoint.http','abc123','http','http://x',100,100)`,
		`INSERT INTO check_results (endpoint_id, success, response_time_ms, timestamp) VALUES (1,1,5,100)`,
		`INSERT INTO cert_monitors (hostname, port, source, status, created_at) VALUES ('example.com',443,'standalone','valid',100)`,
		`INSERT INTO cert_check_results (monitor_id, checked_at) VALUES (1,100)`,
		`INSERT INTO cert_chain_entries (check_result_id, position, subject_cn, issuer_cn, not_before, not_after) VALUES (1,0,'cn','ic',0,200)`,
		`INSERT INTO heartbeats (uuid, name, interval_seconds, grace_seconds, created_at, updated_at) VALUES ('hb-token-1','backup',3600,300,100,100)`,
		`INSERT INTO heartbeat_pings (heartbeat_id, ping_type, source_ip, http_method, timestamp) VALUES (1,'success','1.2.3.4','GET',100)`,
		// alert 1 references the container; alert 2 is resolved_by alert 1 (self-ref)
		`INSERT INTO alerts (source, alert_type, message, entity_type, entity_id, entity_name, fired_at) VALUES ('container','restart','msg','container',1,'web','2026-01-02T15:04:05Z')`,
		`INSERT INTO alerts (source, alert_type, message, entity_type, entity_id, entity_name, resolved_by_id, fired_at) VALUES ('container','restart','msg2','container',1,'web',1,'2026-01-02T16:00:00Z')`,
		`INSERT INTO notification_channels (name, url) VALUES ('slack','http://hook')`,
		`INSERT INTO notification_deliveries (alert_id, channel_id) VALUES (1,1)`,
		`INSERT INTO alert_triggers (name) VALUES ('all')`,
		`INSERT INTO alert_trigger_channels (trigger_id, channel_id) VALUES (1,1)`,
		`INSERT INTO escalation_policies (name, levels_json) VALUES ('p','[]')`,
		`INSERT INTO escalation_runs (policy_id, policy_snapshot_json, alert_id, status) VALUES (1,'{}',1,'active')`,
		`INSERT INTO escalation_deliveries (run_id, level_index, channel_id, status) VALUES (1,0,1,'pending')`,
		`INSERT INTO status_components (display_name, created_at, updated_at) VALUES ('API',100,100)`,
		`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (1,'endpoint',1)`,
		`INSERT INTO incidents (title, severity, created_at, updated_at) VALUES ('down','critical',100,100)`,
		`INSERT INTO maintenance_windows (title, starts_at, ends_at, incident_id, created_at, updated_at) VALUES ('mw',100,200,1,100,100)`,
		`UPDATE incidents SET maintenance_window_id=1 WHERE id=1`,
		`INSERT INTO incident_components (incident_id, component_id) VALUES (1,1)`,
		`INSERT INTO incident_updates (incident_id, status, message, alert_id, created_at) VALUES (1,'investigating','msg',1,100)`,
		`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (1,1)`,
		`INSERT INTO image_update_scans (started_at) VALUES (100)`,
		`INSERT INTO image_updates (scan_id, container_id, container_name, image, current_tag, current_digest, registry, detected_at) VALUES (1,'abc123','web','nginx','1.0','sha','docker',100)`,
		`INSERT INTO swarm_nodes (node_id, hostname, role, status, availability, first_seen_at, last_seen_at, last_status_change_at) VALUES ('node1','h','manager','ready','active',100,100,100)`,
		`INSERT INTO digest_baselines (container_id, image, tag, remote_digest) VALUES ('abc123','nginx','1.0','sha')`,
		`INSERT INTO webhook_subscriptions (id, name, url) VALUES ('wh1','w','http://w')`,
	}
	for _, q := range stmts {
		_, err := db.Exec(q)
		require.NoError(t, err, "seed: %s", q)
	}
}

func scanString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var v sql.NullString
	require.NoError(t, db.QueryRowContext(context.Background(), query, args...).Scan(&v))
	return v.String
}

func TestConvertToUUID(t *testing.T) {
	db := setupLegacyDB(t)
	seedLegacyGraph(t, db)

	err := convertToUUID(context.Background(), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err, "conversion (includes foreign_key_check) must succeed")

	// Row counts preserved.
	require.Equal(t, 1, countRows(t, db, "containers"))
	require.Equal(t, 2, countRows(t, db, "alerts"))
	require.Equal(t, 1, countRows(t, db, "heartbeats"))
	require.Equal(t, 1, countRows(t, db, "cert_chain_entries"))
	require.Equal(t, 1, countRows(t, db, "image_updates"))
	require.Equal(t, 1, countRows(t, db, "incident_components"))

	// Sentinel agent present; the legacy data is attributed to it.
	require.True(t, tableExists(t, db, "agents"))
	require.Equal(t, uid.LocalAgent, scanString(t, db, `SELECT agent_id FROM containers LIMIT 1`))
	require.Equal(t, uid.LocalAgent, scanString(t, db, `SELECT agent_id FROM endpoints LIMIT 1`))

	// Deterministic ids match runtime derivation.
	wantContainer := uid.Container(uid.LocalAgent, "abc123")
	require.Equal(t, wantContainer, scanString(t, db, `SELECT id FROM containers`))
	wantEndpoint := uid.EndpointLabel(uid.LocalAgent, "web", "maintenant.endpoint.http")
	require.Equal(t, wantEndpoint, scanString(t, db, `SELECT id FROM endpoints`))
	wantCert := uid.CertMonitor(uid.LocalAgent, "example.com", 443)
	require.Equal(t, wantCert, scanString(t, db, `SELECT id FROM cert_monitors`))

	// Heartbeat id is the ping token.
	require.Equal(t, "hb-token-1", scanString(t, db, `SELECT id FROM heartbeats`))

	// Polymorphic alert.entity_id remapped onto the container UUID.
	require.Equal(t, wantContainer, scanString(t, db, `SELECT entity_id FROM alerts WHERE message='msg'`))
	// Polymorphic component monitor remapped onto the endpoint UUID.
	require.Equal(t, wantEndpoint, scanString(t, db, `SELECT monitor_id FROM status_component_monitors`))

	// Self-ref: alert 2.resolved_by_id == alert 1.id.
	alert1 := scanString(t, db, `SELECT id FROM alerts WHERE message='msg'`)
	require.Equal(t, alert1, scanString(t, db, `SELECT resolved_by_id FROM alerts WHERE message='msg2'`))

	// Cross-ref incident <-> maintenance both resolved.
	incidentID := scanString(t, db, `SELECT id FROM incidents`)
	maintID := scanString(t, db, `SELECT id FROM maintenance_windows`)
	require.Equal(t, maintID, scanString(t, db, `SELECT maintenance_window_id FROM incidents`))
	require.Equal(t, incidentID, scanString(t, db, `SELECT incident_id FROM maintenance_windows`))

	// DATETIME -> epoch conversion (Go's time.RFC3339 '2026-01-02T15:04:05Z').
	require.Equal(t, "1767366245", scanString(t, db, `SELECT fired_at FROM alerts WHERE message='msg'`))

	// schema_meta marker set.
	require.Equal(t, "uuid-v1", scanString(t, db, `SELECT value FROM schema_meta WHERE key='format'`))

	// One-time: a second call is guarded by schema_meta and changes nothing.
	require.NoError(t, convertToUUID(context.Background(), db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	require.Equal(t, 2, countRows(t, db, "alerts"))
	require.Equal(t, 1, countRows(t, db, "containers"))

	// No legacy tables remain.
	require.False(t, tableExists(t, db, "_old_containers"))
}
