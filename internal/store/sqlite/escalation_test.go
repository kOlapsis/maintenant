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
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/uid"
)

func setupEscalationTestDB(t *testing.T) (*EscalationStore, *sql.DB) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS alerts (
			id TEXT PRIMARY KEY NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			alert_type TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL DEFAULT 'warning',
			status TEXT NOT NULL DEFAULT 'active',
			message TEXT NOT NULL DEFAULT '',
			entity_type TEXT NOT NULL DEFAULT '',
			entity_id TEXT NOT NULL DEFAULT '',
			entity_name TEXT NOT NULL DEFAULT '',
			details TEXT,
			resolved_by_id TEXT,
			fired_at BIGINT NOT NULL DEFAULT 0,
			resolved_at BIGINT,
			acknowledged_at BIGINT,
			acknowledged_by TEXT,
			escalated_at BIGINT,
			created_at BIGINT NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS notification_channels (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL DEFAULT 'webhook',
			url TEXT NOT NULL DEFAULT '',
			headers TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS escalation_policies (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1,
			active_before_downgrade INTEGER NOT NULL DEFAULT 0,
			severities_json TEXT NOT NULL DEFAULT '[]',
			scopes_json TEXT NOT NULL DEFAULT '[]',
			tags_json TEXT NOT NULL DEFAULT '[]',
			levels_json TEXT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT 0,
			created_by TEXT,
			updated_at BIGINT NOT NULL DEFAULT 0,
			updated_by TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_escalation_policies_active ON escalation_policies(active);
		CREATE TABLE IF NOT EXISTS escalation_runs (
			id TEXT PRIMARY KEY NOT NULL,
			policy_id TEXT REFERENCES escalation_policies(id) ON DELETE SET NULL,
			policy_snapshot_json TEXT NOT NULL,
			alert_id TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN (
				'active','paused_by_maintenance','stopped_by_ack','stopped_by_resolution',
				'stopped_by_policy_deletion','stopped_by_policy_disabled',
				'stopped_by_edition_downgrade','exhausted'
			)),
			last_executed_level_index INTEGER NOT NULL DEFAULT -1,
			started_at BIGINT NOT NULL DEFAULT 0,
			ended_at BIGINT,
			next_action_at BIGINT
		);
		CREATE TABLE IF NOT EXISTS escalation_deliveries (
			id TEXT PRIMARY KEY NOT NULL,
			run_id TEXT NOT NULL REFERENCES escalation_runs(id) ON DELETE CASCADE,
			level_index INTEGER NOT NULL,
			channel_id TEXT REFERENCES notification_channels(id) ON DELETE SET NULL,
			status TEXT NOT NULL CHECK (status IN ('pending','sent','failed','abandoned','skipped_maintenance')),
			error TEXT,
			attempt_started_at BIGINT NOT NULL DEFAULT 0,
			sent_at BIGINT
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_escalation_deliveries_run_level
			ON escalation_deliveries(run_id, level_index, channel_id);
	`)
	require.NoError(t, err)

	writer := NewWriter(rawDB, logger)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	writer.Start(ctx)

	d := &DB{db: rawDB, writer: writer, logger: logger}
	store := NewEscalationStore(d)
	return store, rawDB
}

func makeTestPolicy(name string, active bool) *escalation.Policy {
	return &escalation.Policy{
		Name:   name,
		Active: active,
		Filters: escalation.Filters{
			Severities: []string{"critical"},
			Scopes:     []escalation.Scope{{Kind: "container", RefID: "1"}},
			Tags:       []string{"prod"},
		},
		Levels: []escalation.Level{
			{Order: 0, DelaySeconds: 300, ChannelIDs: []string{"1", "2"}},
			{Order: 1, DelaySeconds: 600, ChannelIDs: []string{"3"}},
		},
		CreatedBy: "alice",
		UpdatedBy: "alice",
	}
}

func insertTestAlert(t *testing.T, rawDB *sql.DB) string {
	t.Helper()
	alertID := uid.New()
	_, err := rawDB.ExecContext(context.Background(),
		`INSERT INTO alerts (id, source, alert_type, severity, status, message, entity_type, entity_id, entity_name, fired_at)
		VALUES (?, 'test','t','warning','active','msg','container','1','c1', ?)`,
		alertID, time.Now().Unix(),
	)
	require.NoError(t, err)
	return alertID
}

func TestEscalationStore_InsertSelectPolicy(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	p := makeTestPolicy("on-call critical", true)
	id, err := store.InsertPolicy(ctx, p)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	got, err := store.SelectPolicy(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "on-call critical", got.Name)
	assert.True(t, got.Active)
	assert.Equal(t, []string{"critical"}, got.Filters.Severities)
	assert.Len(t, got.Filters.Scopes, 1)
	assert.Equal(t, "container", got.Filters.Scopes[0].Kind)
	assert.Len(t, got.Levels, 2)
	assert.Equal(t, 300, got.Levels[0].DelaySeconds)
	assert.Equal(t, "alice", got.CreatedBy)
}

func TestEscalationStore_SelectPolicy_NotFound(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	got, err := store.SelectPolicy(ctx, "9999")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEscalationStore_UpdatePolicy(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	p := makeTestPolicy("update-me", true)
	id, err := store.InsertPolicy(ctx, p)
	require.NoError(t, err)

	p.ID = id
	p.Name = "updated-name"
	p.Active = false
	p.UpdatedBy = "bob"
	p.Levels = []escalation.Level{{Order: 0, DelaySeconds: 900, ChannelIDs: []string{"5"}}}

	err = store.UpdatePolicy(ctx, p)
	require.NoError(t, err)

	got, err := store.SelectPolicy(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "updated-name", got.Name)
	assert.False(t, got.Active)
	assert.Equal(t, "bob", got.UpdatedBy)
	assert.Len(t, got.Levels, 1)
	assert.Equal(t, 900, got.Levels[0].DelaySeconds)
}

func TestEscalationStore_DeletePolicy(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	p := makeTestPolicy("to-delete", true)
	id, err := store.InsertPolicy(ctx, p)
	require.NoError(t, err)

	err = store.DeletePolicy(ctx, id)
	require.NoError(t, err)

	got, err := store.SelectPolicy(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEscalationStore_CountActivePolicies(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	_, err := store.InsertPolicy(ctx, makeTestPolicy("active-1", true))
	require.NoError(t, err)
	_, err = store.InsertPolicy(ctx, makeTestPolicy("active-2", true))
	require.NoError(t, err)
	_, err = store.InsertPolicy(ctx, makeTestPolicy("inactive", false))
	require.NoError(t, err)

	n, err := store.CountActivePolicies(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestEscalationStore_BulkDeactivateAndRestore(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	_, _ = store.InsertPolicy(ctx, makeTestPolicy("p1", true))
	_, _ = store.InsertPolicy(ctx, makeTestPolicy("p2", true))

	err := store.BulkDeactivateAllPolicies(ctx)
	require.NoError(t, err)

	n, err := store.CountActivePolicies(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// Verify active_before_downgrade is preserved
	policies, err := store.SelectPolicies(ctx, false)
	require.NoError(t, err)
	for _, p := range policies {
		assert.True(t, p.ActiveBeforeDowngrade, "active_before_downgrade must be set for formerly active policies")
	}

	err = store.BulkRestorePoliciesFromDowngrade(ctx)
	require.NoError(t, err)

	n, err = store.CountActivePolicies(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestEscalationStore_BulkStopActiveRuns(t *testing.T) {
	store, rawDB := setupEscalationTestDB(t)
	ctx := context.Background()

	alertID := insertTestAlert(t, rawDB)

	runID := uid.New()
	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status, started_at)
		VALUES (?, '{}', ?, 'active', ?)`,
		runID, alertID, time.Now().Unix(),
	)
	require.NoError(t, err)

	err = store.BulkStopActiveRuns(ctx, "stopped_by_edition_downgrade", time.Now())
	require.NoError(t, err)

	var status string
	err = rawDB.QueryRowContext(ctx, `SELECT status FROM escalation_runs WHERE id = ?`, runID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "stopped_by_edition_downgrade", status)
}

func TestEscalationStore_PurgeRunsAndDeliveries(t *testing.T) {
	store, rawDB := setupEscalationTestDB(t)
	ctx := context.Background()

	alertID := insertTestAlert(t, rawDB)

	// Run that ended 10 days ago — should be purged
	old := time.Now().Add(-10 * 24 * time.Hour)
	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status, started_at, ended_at)
		VALUES (?, '{}', ?, 'exhausted', ?, ?)`,
		uid.New(), alertID,
		old.Unix(),
		old.Unix(),
	)
	require.NoError(t, err)

	// Run that ended 1 day ago — should survive a 5-day retention cutoff
	recent := time.Now().Add(-1 * 24 * time.Hour)
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status, started_at, ended_at)
		VALUES (?, '{}', ?, 'exhausted', ?, ?)`,
		uid.New(), alertID,
		recent.Unix(),
		recent.Unix(),
	)
	require.NoError(t, err)

	before := time.Now().Add(-5 * 24 * time.Hour)
	err = store.PurgeRunsAndDeliveriesOlderThan(ctx, before)
	require.NoError(t, err)

	var count int
	err = rawDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM escalation_runs`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "only the recent run should survive")
}

func TestEscalationStore_UniqueDeliveryConstraint(t *testing.T) {
	_, rawDB := setupEscalationTestDB(t)
	ctx := context.Background()

	// Insert FK dependencies
	channelID := uid.New()
	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO notification_channels (id, name, type, url, enabled) VALUES (?, 'test-ch', 'webhook', 'http://test', 1)`,
		channelID)
	require.NoError(t, err)

	alertID := insertTestAlert(t, rawDB)

	runID := uid.New()
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO escalation_runs (id, policy_snapshot_json, alert_id, status, started_at)
		VALUES (?, '{}', ?, 'active', ?)`,
		runID, alertID, time.Now().Unix(),
	)
	require.NoError(t, err)

	// First delivery — should succeed
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO escalation_deliveries (id, run_id, level_index, channel_id, status, attempt_started_at)
		VALUES (?, ?, 0, ?, 'pending', ?)`,
		uid.New(), runID, channelID, time.Now().Unix())
	require.NoError(t, err, "first delivery insert should succeed")

	// Duplicate (same run_id, level_index, channel_id) — must fail on UNIQUE constraint
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO escalation_deliveries (id, run_id, level_index, channel_id, status, attempt_started_at)
		VALUES (?, ?, 0, ?, 'sent', ?)`,
		uid.New(), runID, channelID, time.Now().Unix())
	require.Error(t, err, "duplicate delivery must violate unique constraint")
}

func TestEscalationStore_SelectPolicies_ActiveOnly(t *testing.T) {
	store, _ := setupEscalationTestDB(t)
	ctx := context.Background()

	_, _ = store.InsertPolicy(ctx, makeTestPolicy("active", true))
	_, _ = store.InsertPolicy(ctx, makeTestPolicy("inactive", false))

	all, err := store.SelectPolicies(ctx, false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	active, err := store.SelectPolicies(ctx, true)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, "active", active[0].Name)
}
