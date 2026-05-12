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

// setupPreMigrationDB creates a DB with the schema state prior to migration 19
// (notification_channels + routing_rules), then seeds the provided fixture.
func setupPreMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE notification_channels (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			name    TEXT    NOT NULL UNIQUE,
			type    TEXT    NOT NULL DEFAULT 'webhook',
			url     TEXT    NOT NULL DEFAULT '',
			headers TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE routing_rules (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id     INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
			source_filter  TEXT,
			severity_filter TEXT,
			created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_routing_rules_channel ON routing_rules(channel_id);
	`)
	require.NoError(t, err)
	return rawDB
}

// applyMigrationUp reads and executes migrations/19_alert_triggers.up.sql.
func applyMigrationUp(t *testing.T, rawDB *sql.DB) {
	t.Helper()
	sql, err := os.ReadFile("migrations/19_alert_triggers.up.sql")
	require.NoError(t, err, "migration file must be readable")
	_, err = rawDB.ExecContext(context.Background(), string(sql))
	require.NoError(t, err, "migration up must succeed")
}

// applyMigrationDown reads and executes migrations/19_alert_triggers.down.sql.
func applyMigrationDown(t *testing.T, rawDB *sql.DB) {
	t.Helper()
	sql, err := os.ReadFile("migrations/19_alert_triggers.down.sql")
	require.NoError(t, err, "down migration file must be readable")
	_, err = rawDB.ExecContext(context.Background(), string(sql))
	require.NoError(t, err, "migration down must succeed")
}

func countRows(t *testing.T, rawDB *sql.DB, table string) int {
	t.Helper()
	var n int
	err := rawDB.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n)
	require.NoError(t, err)
	return n
}

func tableExists(t *testing.T, rawDB *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := rawDB.QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	require.NoError(t, err)
	return true
}

// Migration test 1: channel + 2 routing_rules → 2 AlertTriggers "Rule for *" + 2 M:N links.
func TestMigration19Up_RoutingRulesConvertedToTriggers(t *testing.T) {
	rawDB := setupPreMigrationDB(t)
	ctx := context.Background()

	var chID int64
	err := rawDB.QueryRowContext(ctx,
		`INSERT INTO notification_channels (name) VALUES ('slack-ops') RETURNING id`).Scan(&chID)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO routing_rules (channel_id, source_filter, severity_filter) VALUES (?, 'container', 'critical'), (?, 'endpoint', 'warning')`,
		chID, chID)
	require.NoError(t, err)

	applyMigrationUp(t, rawDB)

	assert.Equal(t, 2, countRows(t, rawDB, "alert_triggers"), "must have 2 triggers (one per rule)")
	assert.Equal(t, 2, countRows(t, rawDB, "alert_trigger_channels"), "must have 2 M:N links")

	var count int
	err = rawDB.QueryRowContext(ctx,
		`SELECT count(*) FROM alert_triggers WHERE name LIKE 'Rule for %'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "triggers must be named 'Rule for ...'")
}

// Migration test 2: enabled channel without any rule → 1 Default trigger + 1 M:N link.
func TestMigration19Up_EnabledChannelNoRule_DefaultTriggerCreated(t *testing.T) {
	rawDB := setupPreMigrationDB(t)
	ctx := context.Background()

	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO notification_channels (name, enabled) VALUES ('pagerduty', 1)`)
	require.NoError(t, err)

	applyMigrationUp(t, rawDB)

	assert.Equal(t, 1, countRows(t, rawDB, "alert_triggers"), "must have 1 Default trigger")
	assert.Equal(t, 1, countRows(t, rawDB, "alert_trigger_channels"), "must have 1 M:N link")

	var name string
	err = rawDB.QueryRowContext(ctx, `SELECT name FROM alert_triggers LIMIT 1`).Scan(&name)
	require.NoError(t, err)
	assert.Contains(t, name, "Default — all alerts →")
}

// Migration test 3: disabled channel without any rule → 0 Default triggers created.
func TestMigration19Up_DisabledChannelNoRule_NoDefaultTrigger(t *testing.T) {
	rawDB := setupPreMigrationDB(t)
	ctx := context.Background()

	_, err := rawDB.ExecContext(ctx,
		`INSERT INTO notification_channels (name, enabled) VALUES ('silent-ch', 0)`)
	require.NoError(t, err)

	applyMigrationUp(t, rawDB)

	assert.Equal(t, 0, countRows(t, rawDB, "alert_triggers"), "disabled channel must not get a Default trigger")
}

// Migration test 4: routing_rules table must not exist after migration up.
func TestMigration19Up_RoutingRulesTableDropped(t *testing.T) {
	rawDB := setupPreMigrationDB(t)
	applyMigrationUp(t, rawDB)
	assert.False(t, tableExists(t, rawDB, "routing_rules"), "routing_rules table must be dropped after migration up")
}

// Migration test 5: down after up restores routing_rules with "Rule for *" rows.
func TestMigration19DownAfterUp_RestoresRoutingRules(t *testing.T) {
	rawDB := setupPreMigrationDB(t)
	ctx := context.Background()

	var chID int64
	err := rawDB.QueryRowContext(ctx,
		`INSERT INTO notification_channels (name) VALUES ('webhook-a') RETURNING id`).Scan(&chID)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO routing_rules (channel_id, source_filter, severity_filter) VALUES (?, 'container', 'critical')`, chID)
	require.NoError(t, err)

	applyMigrationUp(t, rawDB)
	assert.False(t, tableExists(t, rawDB, "routing_rules"), "pre-condition: routing_rules must be gone")

	applyMigrationDown(t, rawDB)
	assert.True(t, tableExists(t, rawDB, "routing_rules"), "down migration must restore routing_rules")
	assert.False(t, tableExists(t, rawDB, "alert_triggers"), "down migration must drop alert_triggers")
	assert.False(t, tableExists(t, rawDB, "alert_trigger_channels"), "down migration must drop alert_trigger_channels")

	var count int
	err = rawDB.QueryRowContext(ctx, `SELECT count(*) FROM routing_rules`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "down migration must reconstitute the 'Rule for *' row")
}
