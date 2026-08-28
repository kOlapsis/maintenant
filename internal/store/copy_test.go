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

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A copy needs both engines at once: the source is always the local SQLite
// file, the target is always PostgreSQL. So these tests build their own SQLite
// source explicitly rather than going through openTestDB, and skip only when
// no PostgreSQL test server is configured.

// seededSource builds a SQLite install that looks like a real one: agents and
// tokens, a monitor of each kind, alerting configuration, a status page with a
// binary asset and a subscriber, operator decisions — and history in tables
// that must NOT travel, so the test proves the boundary rather than assuming it.
func seededSource(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "source.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(path, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, Migrate(ctx, db, logger))
	db.StartWriter(ctx)
	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})

	rw := db.Reader()
	now := time.Now().Unix()
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := rw.ExecContext(ctx, q, args...)
		require.NoError(t, err, q)
	}

	// Fleet identity.
	agentA, agentB := uid.New(), uid.New()
	for i, id := range []string{agentA, agentB} {
		exec(`INSERT INTO agents (id, public_key, hostname, label, os_arch, agent_version,
			detected_runtime, status, created_at) VALUES (?, ?, ?, ?, 'linux/amd64', '1.0.0', 'docker', 'active', ?)`,
			id, []byte("pubkeyplaceholder12345678901234"), "host-"+string(rune('a'+i)), "edge", now)
	}
	tokenHash := sha256.Sum256([]byte("mnt_enr_copytest0001"))
	exec(`INSERT INTO enrollment_tokens (id, token_hash, token_prefix, created_at, expires_at)
		VALUES (?, ?, 'mnt_enr', ?, ?)`,
		hex.EncodeToString(tokenHash[:])[:16], hex.EncodeToString(tokenHash[:]), now, now+3600)

	// Reference anchor and its thresholds.
	containerID := uid.New()
	exec(`INSERT INTO containers (id, agent_id, external_id, name, image, state,
		first_seen_at, last_state_change_at) VALUES (?, ?, 'ext-1', 'web', 'nginx:1', 'running', ?, ?)`,
		containerID, agentA, now, now)
	exec(`INSERT INTO resource_alert_configs (id, container_id, cpu_threshold, mem_threshold,
		created_at, updated_at) VALUES (?, ?, 80.5, 90.0, ?, ?)`, uid.New(), containerID, now, now)

	// One monitor of each declared kind.
	endpointID, heartbeatID, certID := uid.New(), uid.New(), uid.New()
	exec(`INSERT INTO endpoints (id, agent_id, container_name, label_key, external_id, endpoint_type,
		target, status, first_seen_at, last_seen_at, name)
		VALUES (?, ?, '', 'manual', 'ep-1', 'http', 'https://example.com', 'up', ?, ?, 'site')`,
		endpointID, agentA, now, now)
	exec(`INSERT INTO heartbeats (id, agent_id, name, interval_seconds, grace_seconds, created_at, updated_at)
		VALUES (?, ?, 'nightly', 86400, 600, ?, ?)`, heartbeatID, agentA, now, now)
	exec(`INSERT INTO cert_monitors (id, agent_id, hostname, port, source, status,
		warning_thresholds_json, created_at, server_name)
		VALUES (?, ?, 'example.com', 443, 'standalone', 'valid', '[30,14,7]', ?, '')`, certID, agentA, now)

	// Alerting configuration, channel secret included.
	channelID, triggerID := uid.New(), uid.New()
	exec(`INSERT INTO notification_channels (id, name, type, url, enabled, created_at, updated_at)
		VALUES (?, 'ops', 'webhook', 'https://hooks.example/secret', 1, ?, ?)`, channelID, now, now)
	exec(`INSERT INTO alert_triggers (id, name, filter_severities, enabled, created_at, updated_at)
		VALUES (?, 'page ops', '["critical"]', 1, ?, ?)`, triggerID, now, now)
	exec(`INSERT INTO alert_trigger_channels (trigger_id, channel_id) VALUES (?, ?)`, triggerID, channelID)
	exec(`INSERT INTO silence_rules (id, entity_type, entity_id, source, reason, starts_at,
		duration_seconds, created_at) VALUES (?, 'container', ?, 'manual', 'deploy', ?, 3600, ?)`,
		uid.New(), containerID, now, now)
	exec(`INSERT INTO escalation_policies (id, name, active, levels_json, created_at, updated_at)
		VALUES (?, 'oncall', 1, '[]', ?, ?)`, uid.New(), now, now)
	exec(`INSERT INTO webhook_subscriptions (id, name, url, secret, event_types, is_active, created_at)
		VALUES (?, 'sub', 'https://hooks.example/sub', 'shhh', '["alert.fired"]', 1, ?)`, uid.New(), now)

	// Status page, with a binary asset and a subscriber whose consent cannot
	// be rebuilt.
	componentID := uid.New()
	exec(`INSERT INTO status_components (id, composition_mode, display_name, display_order,
		visible, created_at, updated_at) VALUES (?, 'explicit', 'Web', 0, 1, ?, ?)`, componentID, now, now)
	exec(`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id)
		VALUES (?, 'endpoint', ?)`, componentID, endpointID)
	// The settings row is a singleton the schema already installs.
	exec(`UPDATE status_page_settings SET title = 'Status', updated_at = ? WHERE id = 1`, now)
	exec(`INSERT INTO status_page_assets (role, mime, bytes, byte_size, updated_at)
		VALUES ('logo', 'image/png', ?, 6, ?)`, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}, now)
	exec(`INSERT INTO status_page_faq_items (id, position, question, answer_md, answer_html, created_at, updated_at)
		VALUES (?, 0, 'Why?', 'Because.', '<p>Because.</p>', ?, ?)`, uid.New(), now, now)
	exec(`INSERT INTO status_page_footer_links (id, position, label, url, created_at, updated_at)
		VALUES (?, 0, 'Docs', 'https://docs.example', ?, ?)`, uid.New(), now, now)
	exec(`INSERT INTO status_subscribers (id, email, confirmed, unsub_token, created_at)
		VALUES (?, 'ops@example.com', 1, 'unsub-token-1', ?)`, uid.New(), now)

	// Public communication and planned maintenance.
	incidentID, windowID := uid.New(), uid.New()
	exec(`INSERT INTO incidents (id, title, severity, status, created_at, updated_at)
		VALUES (?, 'Partial outage', 'minor', 'resolved', ?, ?)`, incidentID, now, now)
	exec(`INSERT INTO incident_updates (id, incident_id, status, message, created_at)
		VALUES (?, ?, 'resolved', 'Fixed.', ?)`, uid.New(), incidentID, now)
	exec(`INSERT INTO incident_components (incident_id, component_id) VALUES (?, ?)`, incidentID, componentID)
	exec(`INSERT INTO maintenance_windows (id, title, description, starts_at, ends_at, active, created_at, updated_at)
		VALUES (?, 'Upgrade', 'db upgrade', ?, ?, 1, ?, ?)`, windowID, now+3600, now+7200, now, now)
	exec(`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`, windowID, componentID)

	// Operator decisions.
	exec(`INSERT INTO update_exclusions (id, pattern, pattern_type, created_at)
		VALUES (?, 'nginx:*', 'image', ?)`, uid.New(), now)
	exec(`INSERT INTO version_pins (id, container_id, image, pinned_tag, pinned_digest, reason, pinned_at)
		VALUES (?, ?, 'nginx', '1.25', 'sha256:abc', 'stability', ?)`, uid.New(), containerID, now)
	exec(`INSERT INTO risk_acknowledgments (id, container_external_id, finding_type, finding_key,
		reason, acknowledged_at) VALUES (?, 'ext-1', 'cve', 'CVE-2026-1', 'not exploitable', ?)`, uid.New(), now)

	// History that must NOT travel.
	exec(`INSERT INTO check_results (id, endpoint_id, success, response_time_ms, timestamp)
		VALUES (?, ?, 1, 42, ?)`, uid.New(), endpointID, now)
	exec(`INSERT INTO heartbeat_pings (id, heartbeat_id, ping_type, source_ip, http_method, timestamp)
		VALUES (?, ?, 'success', '127.0.0.1', 'POST', ?)`, uid.New(), heartbeatID, now)
	exec(`INSERT INTO state_transitions (id, container_id, previous_state, new_state, timestamp)
		VALUES (?, ?, 'created', 'running', ?)`, uid.New(), containerID, now)
	exec(`INSERT INTO resource_snapshots (id, container_id, agent_id, cpu_percent, mem_used, mem_limit,
		net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		VALUES (?, ?, ?, 12.5, 100, 200, 0, 0, 0, 0, ?)`, uid.New(), containerID, agentA, now)
	exec(`INSERT INTO alerts (id, source, alert_type, severity, status, message, entity_type,
		entity_id, entity_name, fired_at)
		VALUES (?, 'resource', 'threshold', 'critical', 'active', 'CPU high', 'container', ?, 'web', ?)`,
		uid.New(), containerID, now)

	return db, path
}

func openEmptyTarget(t *testing.T) *sql.DB {
	t.Helper()
	dsn := createTestDatabase(t, testAdminDSN(t))
	dst, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dst.Close() })
	return dst
}

func fileChecksum(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestCopy_CarriesWhatCannotBeRebuilt is SC-009 and FR-024/FR-025: everything
// the fleet cannot rebuild arrives, counted table by table, and nothing else
// does. The target is then a database the product can start on, with no
// migration pending.
func TestCopy_CarriesWhatCannotBeRebuilt(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)
	ctx := context.Background()

	var out bytes.Buffer
	report, err := Copy(ctx, src.ReadDB(), dst, &out, func(Plan) bool { return true })
	require.NoError(t, err)
	require.True(t, report.Verified)

	// Every carried table arrived whole.
	require.Len(t, report.Copied, len(carriedTables))
	for _, c := range report.Copied {
		var srcCount int64
		require.NoError(t, src.ReadDB().QueryRow("SELECT count(*) FROM "+c.Table).Scan(&srcCount))
		assert.Equal(t, srcCount, c.Rows, "%s must arrive whole", c.Table)
	}

	// The tables that rebuild themselves stayed behind.
	for _, group := range leftBehindGroups {
		for _, table := range group.Tables {
			if table == "schema_meta" {
				continue // not in the PostgreSQL schema at all
			}
			var n int64
			require.NoError(t, dst.QueryRow("SELECT count(*) FROM "+table).Scan(&n),
				"%s must exist in the target", table)
			assert.Zero(t, n, "%s must not travel (FR-025)", table)
		}
	}

	// Spot-check the data that matters most: identities, a channel secret, a
	// binary asset, a subscriber's consent.
	var agents int64
	require.NoError(t, dst.QueryRow("SELECT count(*) FROM agents WHERE id != $1", uid.LocalAgent).Scan(&agents))
	assert.Equal(t, int64(2), agents, "the fleet identity travels")

	var channelURL string
	require.NoError(t, dst.QueryRow("SELECT url FROM notification_channels").Scan(&channelURL))
	assert.Equal(t, "https://hooks.example/secret", channelURL, "channel secrets travel with their channel")

	var webhookSecret string
	require.NoError(t, dst.QueryRow("SELECT secret FROM webhook_subscriptions").Scan(&webhookSecret))
	assert.Equal(t, "shhh", webhookSecret)

	var assetBytes []byte
	require.NoError(t, dst.QueryRow("SELECT bytes FROM status_page_assets").Scan(&assetBytes))
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}, assetBytes, "BLOB becomes BYTEA intact")

	var email string
	require.NoError(t, dst.QueryRow("SELECT email FROM status_subscribers").Scan(&email))
	assert.Equal(t, "ops@example.com", email)

	// The target is at the head: a normal start finds nothing pending.
	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	var version uint
	require.NoError(t, dst.QueryRow("SELECT version FROM schema_migrations").Scan(&version))
	assert.Equal(t, head, version)
}

// TestCopy_AnnouncesBeforeWriting is FR-026 and SC-010: the operator is told
// what stays behind before anything is written, not after.
func TestCopy_AnnouncesBeforeWriting(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)

	var out bytes.Buffer
	announcedBeforeWrite := false
	_, err := Copy(context.Background(), src.ReadDB(), dst, &out, func(p Plan) bool {
		// Called before the transaction: nothing exists in the target yet.
		var n int
		require.NoError(t, dst.QueryRow(`SELECT count(*) FROM information_schema.tables
			WHERE table_schema = current_schema()`).Scan(&n))
		assert.Zero(t, n, "the plan is announced before a single table is created")
		assert.NotEmpty(t, p.Carried)
		announcedBeforeWrite = true
		return true
	})
	require.NoError(t, err)
	require.True(t, announcedBeforeWrite, "confirm must be called")

	text := out.String()
	for _, table := range []string{"agents", "enrollment_tokens", "status_subscribers"} {
		assert.Contains(t, text, table, "the announcement names what travels")
	}
	assert.Contains(t, text, "resource_snapshots", "and what does not")
	assert.Contains(t, text, "unacknowledged", "and the acknowledgement consequence")
	assert.Contains(t, text, "Curves start from zero", "and the history consequence")
}

// TestCopy_RefusedByOperator leaves the target untouched.
func TestCopy_RefusedByOperator(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)

	_, err := Copy(context.Background(), src.ReadDB(), dst, nil, func(Plan) bool { return false })
	require.ErrorIs(t, err, ErrCopyRefused)

	var n int
	require.NoError(t, dst.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()`).Scan(&n))
	assert.Zero(t, n, "refusing writes nothing at all")
}

// TestCopy_RefusesNonEmptyTarget is FR-027: refused before writing anything.
// Merging would need conflict rules nothing can settle correctly.
func TestCopy_RefusesNonEmptyTarget(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)
	ctx := context.Background()

	_, err := dst.ExecContext(ctx, "CREATE TABLE something (id TEXT)")
	require.NoError(t, err)

	confirmed := false
	_, err = Copy(ctx, src.ReadDB(), dst, nil, func(Plan) bool { confirmed = true; return true })
	require.ErrorIs(t, err, ErrTargetNotEmpty)
	assert.False(t, confirmed, "refused before even asking")

	var n int
	require.NoError(t, dst.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()`).Scan(&n))
	assert.Equal(t, 1, n, "the target keeps exactly what it had")
}

// TestCopy_RefusesSourceBehindHead keeps the shapes matched: a source that has
// not been migrated would carry a schema the target does not have.
func TestCopy_RefusesSourceBehindHead(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)

	_, err := src.ReadDB().Exec("UPDATE schema_migrations SET version = 1")
	require.NoError(t, err)

	_, err = Copy(context.Background(), src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.ErrorIs(t, err, ErrSourceBehind)
	assert.Contains(t, err.Error(), "start the binary on the source once")
}

// TestCopy_FailureLeavesBothSidesIntact is FR-028, the property that makes a
// retry safe: a failure half-way rolls the schema back with the data, so the
// target is empty again and the source was never written to.
func TestCopy_FailureLeavesBothSidesIntact(t *testing.T) {
	src, path := seededSource(t)
	dst := openEmptyTarget(t)
	ctx := context.Background()

	// Close the writer's view of the source so the file settles, then take its
	// checksum: the source must come out byte-identical.
	before := fileChecksum(t, path)

	// Break the copy in the middle: drop a table the source needs to read,
	// after several tables have already been written inside the transaction.
	_, err := src.ReadDB().Exec("DROP TABLE status_subscribers")
	require.NoError(t, err)

	_, err = Copy(ctx, src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.Error(t, err, "the copy must fail")

	// The target is empty again: the schema went back with the data.
	var n int
	require.NoError(t, dst.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()`).Scan(&n))
	assert.Zero(t, n, "a failed copy leaves no intermediate state (FR-028)")

	// The source is untouched by the copy itself.
	assert.NotEmpty(t, before)
	var agents int
	require.NoError(t, src.ReadDB().QueryRow("SELECT count(*) FROM agents").Scan(&agents))
	assert.Equal(t, 3, agents, "source data intact (2 agents + the local sentinel)")
}

// TestCopy_RetryAfterFailureSucceeds closes the loop: because the failure left
// nothing behind, running it again works.
func TestCopy_RetryAfterFailureSucceeds(t *testing.T) {
	src, _ := seededSource(t)
	dst := openEmptyTarget(t)
	ctx := context.Background()

	// First attempt fails at the very last table.
	_, err := src.ReadDB().Exec("DROP TABLE risk_acknowledgments")
	require.NoError(t, err)
	_, err = Copy(ctx, src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.Error(t, err)

	// Put the source back the way it was and retry.
	_, err = src.ReadDB().Exec(`CREATE TABLE risk_acknowledgments (
		id TEXT PRIMARY KEY NOT NULL,
		container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
		finding_type TEXT NOT NULL,
		finding_id TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	report, err := Copy(ctx, src.ReadDB(), dst, nil, func(Plan) bool { return true })
	require.NoError(t, err, "the retry works because the first attempt left nothing")
	assert.True(t, report.Verified)
}
