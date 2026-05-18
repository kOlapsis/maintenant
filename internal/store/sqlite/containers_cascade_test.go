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
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupPreCascadeDB reproduces the containers + child tables schema as it
// existed before migration 21 (no ON DELETE CASCADE on child FKs).
func setupPreCascadeDB(t *testing.T) *sql.DB {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE containers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			external_id TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			image TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'exited',
			archived INTEGER NOT NULL DEFAULT 0,
			archived_at INTEGER,
			first_seen_at INTEGER NOT NULL DEFAULT 0,
			last_state_change_at INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE state_transitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id INTEGER NOT NULL REFERENCES containers(id),
			previous_state TEXT NOT NULL DEFAULT '',
			new_state TEXT NOT NULL DEFAULT '',
			previous_health TEXT,
			new_health TEXT,
			exit_code INTEGER,
			log_snippet TEXT,
			timestamp INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE resource_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id INTEGER NOT NULL REFERENCES containers(id),
			cpu_percent REAL NOT NULL DEFAULT 0,
			mem_used INTEGER NOT NULL DEFAULT 0,
			mem_limit INTEGER NOT NULL DEFAULT 0,
			net_rx_bytes INTEGER NOT NULL DEFAULT 0,
			net_tx_bytes INTEGER NOT NULL DEFAULT 0,
			block_read_bytes INTEGER NOT NULL DEFAULT 0,
			block_write_bytes INTEGER NOT NULL DEFAULT 0,
			timestamp INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE resource_alert_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			container_id INTEGER NOT NULL UNIQUE REFERENCES containers(id),
			cpu_threshold REAL NOT NULL DEFAULT 90.0,
			mem_threshold REAL NOT NULL DEFAULT 90.0,
			enabled INTEGER NOT NULL DEFAULT 0,
			alert_state TEXT NOT NULL DEFAULT 'normal',
			cpu_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
			mem_consecutive_breaches INTEGER NOT NULL DEFAULT 0,
			last_alerted_at INTEGER,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
	`)
	require.NoError(t, err)
	return rawDB
}

func applyMigration21Up(t *testing.T, rawDB *sql.DB) {
	t.Helper()
	sqlBytes, err := os.ReadFile("migrations/21_containers_cascade_delete.up.sql")
	require.NoError(t, err, "migration file must be readable")
	_, err = rawDB.ExecContext(context.Background(), string(sqlBytes))
	require.NoError(t, err, "migration 21 up must succeed")
}

func applyMigration21Down(t *testing.T, rawDB *sql.DB) {
	t.Helper()
	sqlBytes, err := os.ReadFile("migrations/21_containers_cascade_delete.down.sql")
	require.NoError(t, err, "down migration file must be readable")
	_, err = rawDB.ExecContext(context.Background(), string(sqlBytes))
	require.NoError(t, err, "migration 21 down must succeed")
}

// Issue #25: before migration 21, deleting an archived container fails because
// its children (transitions, snapshots, alert configs) reference it via FKs
// without ON DELETE CASCADE.
func TestMigration21_BeforeUp_DeleteContainerFailsWithFKError(t *testing.T) {
	rawDB := setupPreCascadeDB(t)
	ctx := context.Background()
	seedContainerWithChildren(t, rawDB)

	_, err := rawDB.ExecContext(ctx, `DELETE FROM containers WHERE id = 1`)
	require.Error(t, err, "delete must fail under the pre-migration schema")
	assert.Contains(t, err.Error(), "FOREIGN KEY constraint failed")
}

// After migration 21, deleting a container cascades to all three child tables.
func TestMigration21Up_DeleteContainerCascadesChildren(t *testing.T) {
	rawDB := setupPreCascadeDB(t)
	ctx := context.Background()
	seedContainerWithChildren(t, rawDB)

	applyMigration21Up(t, rawDB)

	// Pre-condition: children survived the migration.
	assert.Equal(t, 2, countRows(t, rawDB, "state_transitions"))
	assert.Equal(t, 3, countRows(t, rawDB, "resource_snapshots"))
	assert.Equal(t, 1, countRows(t, rawDB, "resource_alert_configs"))

	_, err := rawDB.ExecContext(ctx, `DELETE FROM containers WHERE id = 1`)
	require.NoError(t, err, "delete must succeed thanks to ON DELETE CASCADE")

	assert.Equal(t, 0, countRows(t, rawDB, "containers"))
	assert.Equal(t, 0, countRows(t, rawDB, "state_transitions"), "transitions must cascade")
	assert.Equal(t, 0, countRows(t, rawDB, "resource_snapshots"), "snapshots must cascade")
	assert.Equal(t, 0, countRows(t, rawDB, "resource_alert_configs"), "alert config must cascade")
}

// The down migration restores the strict (non-cascading) FK behavior.
func TestMigration21Down_RestoresStrictFK(t *testing.T) {
	rawDB := setupPreCascadeDB(t)
	ctx := context.Background()
	seedContainerWithChildren(t, rawDB)

	applyMigration21Up(t, rawDB)
	applyMigration21Down(t, rawDB)

	_, err := rawDB.ExecContext(ctx, `DELETE FROM containers WHERE id = 1`)
	require.Error(t, err, "delete must fail again after down migration")
	assert.Contains(t, err.Error(), "FOREIGN KEY constraint failed")
}

func seedContainerWithChildren(t *testing.T, rawDB *sql.DB) {
	t.Helper()
	ctx := context.Background()

	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO containers (id, external_id, name, image, state, archived, archived_at)
		 VALUES (1, 'ext-1', 'web', 'nginx:1.27', 'exited', 1, 1000)`)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO state_transitions (container_id, previous_state, new_state, timestamp)
		 VALUES (1, 'running', 'exited', 1000), (1, 'exited', 'archived', 2000)`)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO resource_snapshots (container_id, timestamp)
		 VALUES (1, 100), (1, 200), (1, 300)`)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO resource_alert_configs (container_id, created_at, updated_at)
		 VALUES (1, 100, 100)`)
	require.NoError(t, err)
}