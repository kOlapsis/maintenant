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

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func setupTriggerTestDB(t *testing.T) (*TriggerStoreImpl, *sql.DB) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	rawDB, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS notification_channels (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			name    TEXT    NOT NULL UNIQUE,
			type    TEXT    NOT NULL DEFAULT 'webhook',
			url     TEXT    NOT NULL DEFAULT '',
			headers TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS alert_triggers (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			name              TEXT    NOT NULL UNIQUE,
			filter_severities TEXT    NOT NULL DEFAULT '',
			filter_sources    TEXT    NOT NULL DEFAULT '',
			filter_scopes     TEXT    NOT NULL DEFAULT '',
			filter_tags       TEXT    NOT NULL DEFAULT '',
			enabled           INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
			created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at        TEXT    NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_alert_triggers_enabled ON alert_triggers(enabled);
		CREATE TABLE IF NOT EXISTS alert_trigger_channels (
			trigger_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			PRIMARY KEY (trigger_id, channel_id),
			FOREIGN KEY (trigger_id) REFERENCES alert_triggers(id) ON DELETE CASCADE,
			FOREIGN KEY (channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_atc_channel ON alert_trigger_channels(channel_id);
	`)
	require.NoError(t, err)

	writer := NewWriter(rawDB, logger)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	writer.Start(ctx)

	d := &DB{db: rawDB, writer: writer, logger: logger}
	return NewTriggerStore(d), rawDB
}

func seedChannelForTrigger(t *testing.T, rawDB *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	err := rawDB.QueryRowContext(context.Background(),
		`INSERT INTO notification_channels (name) VALUES (?) RETURNING id`, name).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestTriggerStore_InsertAndGet(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "slack-ops")

	trig := &alert.AlertTrigger{
		Name:             "Critical containers",
		FilterSeverities: "critical",
		FilterSources:    "container",
		Enabled:          true,
		ChannelIDs:       []int64{chID},
	}
	id, err := store.InsertTrigger(ctx, trig)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	got, err := store.GetTrigger(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Critical containers", got.Name)
	assert.Equal(t, "critical", got.FilterSeverities)
	assert.Equal(t, "container", got.FilterSources)
	assert.True(t, got.Enabled)
	assert.Equal(t, []int64{chID}, got.ChannelIDs)
}

func TestTriggerStore_GetNotFound(t *testing.T) {
	store, _ := setupTriggerTestDB(t)
	got, err := store.GetTrigger(context.Background(), 99999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTriggerStore_ListTriggers(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "list-ch")
	for _, name := range []string{"Alpha", "Beta"} {
		_, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: name, Enabled: true, ChannelIDs: []int64{chID}})
		require.NoError(t, err)
	}

	triggers, err := store.ListTriggers(ctx)
	require.NoError(t, err)
	assert.Len(t, triggers, 2)
}

func TestTriggerStore_ListEnabledTriggersFiltersDisabled(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "enabled-ch")
	_, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "Active", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)
	_, err = store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "Inactive", Enabled: false, ChannelIDs: []int64{chID}})
	require.NoError(t, err)

	triggers, err := store.ListEnabledTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, triggers, 1)
	assert.Equal(t, "Active", triggers[0].Name)
}

func TestTriggerStore_Update(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID1 := seedChannelForTrigger(t, rawDB, "upd-ch1")
	chID2 := seedChannelForTrigger(t, rawDB, "upd-ch2")

	trig := &alert.AlertTrigger{Name: "Original", Enabled: true, ChannelIDs: []int64{chID1}}
	id, err := store.InsertTrigger(ctx, trig)
	require.NoError(t, err)

	trig.ID = id
	trig.Name = "Updated"
	trig.FilterSeverities = "warning"
	trig.ChannelIDs = []int64{chID1, chID2}
	require.NoError(t, store.UpdateTrigger(ctx, trig))

	got, err := store.GetTrigger(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "warning", got.FilterSeverities)
	assert.ElementsMatch(t, []int64{chID1, chID2}, got.ChannelIDs)
}

func TestTriggerStore_Delete(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "del-ch")
	id, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "ToDelete", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)

	require.NoError(t, store.DeleteTrigger(ctx, id))

	got, err := store.GetTrigger(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestTriggerStore_SetChannels_AtomicReplace(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID1 := seedChannelForTrigger(t, rawDB, "set-ch1")
	chID2 := seedChannelForTrigger(t, rawDB, "set-ch2")
	chID3 := seedChannelForTrigger(t, rawDB, "set-ch3")

	id, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "SetCh", Enabled: true, ChannelIDs: []int64{chID1, chID2}})
	require.NoError(t, err)

	require.NoError(t, store.SetChannels(ctx, id, []int64{chID3}))

	got, err := store.GetTrigger(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, []int64{chID3}, got.ChannelIDs)
}

func TestTriggerStore_ListTriggersForChannel(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "for-ch")
	_, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "T1", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)
	_, err = store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "T2", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)

	triggers, err := store.ListTriggersForChannel(ctx, chID)
	require.NoError(t, err)
	assert.Len(t, triggers, 2)
}

func TestTriggerStore_CascadeDeleteTrigger_RemovesLinks(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "casc-trig-ch")
	id, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "CascT", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)

	var count int
	err = rawDB.QueryRowContext(ctx, `SELECT count(*) FROM alert_trigger_channels WHERE trigger_id = ?`, id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	require.NoError(t, store.DeleteTrigger(ctx, id))

	err = rawDB.QueryRowContext(ctx, `SELECT count(*) FROM alert_trigger_channels WHERE trigger_id = ?`, id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestTriggerStore_CascadeDeleteChannel_RemovesLinks(t *testing.T) {
	store, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()

	chID := seedChannelForTrigger(t, rawDB, "casc-chan-ch")
	id, err := store.InsertTrigger(ctx, &alert.AlertTrigger{Name: "CascC", Enabled: true, ChannelIDs: []int64{chID}})
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, chID)
	require.NoError(t, err)

	var count int
	err = rawDB.QueryRowContext(ctx, `SELECT count(*) FROM alert_trigger_channels WHERE trigger_id = ?`, id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "link must be removed when channel is deleted")

	got, err := store.GetTrigger(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.ChannelIDs, "trigger must survive with empty channel list")
}
