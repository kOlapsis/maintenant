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
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/uid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMaintenanceTestDB(t *testing.T) (*MaintenanceStoreImpl, *sql.DB) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE maintenance_windows (
			id TEXT PRIMARY KEY NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			starts_at INTEGER NOT NULL,
			ends_at INTEGER NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			incident_id TEXT,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX idx_maintenance_windows_schedule ON maintenance_windows(starts_at, ends_at);

		CREATE TABLE status_components (
			id TEXT PRIMARY KEY NOT NULL,
			composition_mode TEXT NOT NULL DEFAULT 'explicit',
			match_all_type TEXT,
			display_name TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE maintenance_components (
			maintenance_id TEXT NOT NULL REFERENCES maintenance_windows(id) ON DELETE CASCADE,
			component_id TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
			PRIMARY KEY (maintenance_id, component_id)
		);

		CREATE TABLE status_component_monitors (
			component_id TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
			monitor_type TEXT NOT NULL,
			monitor_id TEXT NOT NULL,
			PRIMARY KEY (component_id, monitor_type, monitor_id)
		);
		CREATE INDEX idx_status_component_monitors_lookup ON status_component_monitors(monitor_type, monitor_id);
	`)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	writer := NewWriter(rawDB, logger)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	writer.Start(ctx)

	d := &DB{db: rawDB, writer: writer, logger: logger}
	return NewMaintenanceStore(d), rawDB
}

// insertWindow inserts a maintenance window with the given start/end (unix seconds).
func insertWindow(t *testing.T, db *sql.DB, startsAt, endsAt int64) string {
	t.Helper()
	id := uid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO maintenance_windows (id, title, description, starts_at, ends_at, active, created_at, updated_at)
		VALUES (?, 'Test', '', ?, ?, 0, 0, 0)`,
		id, startsAt, endsAt,
	)
	require.NoError(t, err)
	return id
}

// insertExplicitComponent inserts a status_component with composition_mode='explicit'.
func insertExplicitComponent(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO status_components (id, composition_mode, match_all_type, display_name) VALUES (?, 'explicit', NULL, 'Test')`,
		id,
	)
	require.NoError(t, err)
	return id
}

// insertMatchAllComponent inserts a status_component with composition_mode='match-all'.
func insertMatchAllComponent(t *testing.T, db *sql.DB, matchAllType string) string {
	t.Helper()
	id := uid.New()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO status_components (id, composition_mode, match_all_type, display_name) VALUES (?, 'match-all', ?, 'Test')`,
		id, matchAllType,
	)
	require.NoError(t, err)
	return id
}

func linkWindowComponent(t *testing.T, db *sql.DB, windowID, componentID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`,
		windowID, componentID,
	)
	require.NoError(t, err)
}

func linkComponentMonitor(t *testing.T, db *sql.DB, componentID string, monitorType string, monitorID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, ?, ?)`,
		componentID, monitorType, monitorID,
	)
	require.NoError(t, err)
}

func TestIsEntitySuppressed(t *testing.T) {
	ctx := context.Background()
	// Fixed clock: "now" is 2026-01-15T12:00:00Z
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	t.Run("explicit_match", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		// Active window covers now
		wid := insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)
		cid := insertExplicitComponent(t, db)
		linkWindowComponent(t, db, wid, cid)
		linkComponentMonitor(t, db, cid, "endpoint", "42")

		matched, windowID, endsAt, err := store.IsEntitySuppressed(ctx, "endpoint", "42", now)
		require.NoError(t, err)
		assert.True(t, matched)
		assert.Equal(t, wid, windowID)
		assert.Equal(t, now.Unix()+3600, endsAt.Unix())
	})

	t.Run("match_all_by_type", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		wid := insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)
		cid := insertMatchAllComponent(t, db, "container")
		linkWindowComponent(t, db, wid, cid)
		// No status_component_monitors row needed for match-all

		// Same type → match
		matched, _, _, err := store.IsEntitySuppressed(ctx, "container", "999", now)
		require.NoError(t, err)
		assert.True(t, matched)

		// Different type → no match
		matched2, _, _, err2 := store.IsEntitySuppressed(ctx, "endpoint", "999", now)
		require.NoError(t, err2)
		assert.False(t, matched2)
	})

	t.Run("past_window", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		// Window ended 1 hour ago
		wid := insertWindow(t, db, now.Unix()-7200, now.Unix()-3600)
		cid := insertExplicitComponent(t, db)
		linkWindowComponent(t, db, wid, cid)
		linkComponentMonitor(t, db, cid, "container", "42")

		matched, _, _, err := store.IsEntitySuppressed(ctx, "container", "42", now)
		require.NoError(t, err)
		assert.False(t, matched)
	})

	t.Run("future_window", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		// Window starts 1 hour from now
		wid := insertWindow(t, db, now.Unix()+3600, now.Unix()+7200)
		cid := insertExplicitComponent(t, db)
		linkWindowComponent(t, db, wid, cid)
		linkComponentMonitor(t, db, cid, "container", "42")

		matched, _, _, err := store.IsEntitySuppressed(ctx, "container", "42", now)
		require.NoError(t, err)
		assert.False(t, matched)
	})

	t.Run("window_no_components", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		// Window with no components linked
		_ = insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)

		matched, _, _, err := store.IsEntitySuppressed(ctx, "container", "42", now)
		require.NoError(t, err)
		assert.False(t, matched)
	})

	t.Run("explicit_component_no_monitors", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		wid := insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)
		cid := insertExplicitComponent(t, db)
		linkWindowComponent(t, db, wid, cid)
		// No monitors linked to explicit component

		matched, _, _, err := store.IsEntitySuppressed(ctx, "container", "42", now)
		require.NoError(t, err)
		assert.False(t, matched)
	})

	t.Run("two_windows_one_covers", func(t *testing.T) {
		store, db := setupMaintenanceTestDB(t)
		// Window 1: covers container:42
		wid1 := insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)
		cid1 := insertExplicitComponent(t, db)
		linkWindowComponent(t, db, wid1, cid1)
		linkComponentMonitor(t, db, cid1, "container", "42")

		// Window 2: active but no components
		_ = insertWindow(t, db, now.Unix()-3600, now.Unix()+3600)

		matched, windowID, _, err := store.IsEntitySuppressed(ctx, "container", "42", now)
		require.NoError(t, err)
		assert.True(t, matched)
		assert.Equal(t, wid1, windowID)
	})
}
