// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 043-public-surfaces-hardening, US2 (C2): FR-010, FR-011, scenario 4 and 5, SC-002.

// schemaBeforePingToken is the last migration of the release that kept the id as the ping token.
const schemaBeforePingToken = 41

func newStoredHeartbeat(name, token string) *heartbeat.Heartbeat {
	return &heartbeat.Heartbeat{ID: uid.New(), PingToken: token, Name: name, IntervalSeconds: 300, GraceSeconds: 60}
}

func TestHeartbeatStore_ResolvesByPingTokenOnly(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	hs := NewHeartbeatStore(db)

	a := newStoredHeartbeat("nightly backup", "tok-a-"+uid.New())
	b := newStoredHeartbeat("hourly sync", "tok-b-"+uid.New())
	for _, h := range []*heartbeat.Heartbeat{a, b} {
		_, err := hs.CreateHeartbeat(ctx, h)
		require.NoError(t, err)
	}

	for _, h := range []*heartbeat.Heartbeat{a, b} {
		byID, err := hs.GetHeartbeatByID(ctx, h.ID)
		require.NoError(t, err)
		require.NotNil(t, byID)
		assert.Equal(t, h.PingToken, byID.PingToken, "%s: the stored token is read back", h.Name)

		byToken, err := hs.GetHeartbeatByPingToken(ctx, h.PingToken)
		require.NoError(t, err)
		require.NotNil(t, byToken, "%s: the token resolves the heartbeat", h.Name)
		assert.Equal(t, h.ID, byToken.ID)
		assert.Equal(t, h.PingToken, byToken.PingToken)

		byWrongKey, err := hs.GetHeartbeatByPingToken(ctx, h.ID)
		require.NoError(t, err)
		assert.Nil(t, byWrongKey, "%s: the id is not a token", h.Name)
	}

	unknown, err := hs.GetHeartbeatByPingToken(ctx, uid.New())
	require.NoError(t, err)
	assert.Nil(t, unknown)

	listed, err := hs.ListHeartbeats(ctx, heartbeat.ListHeartbeatsOpts{})
	require.NoError(t, err)
	tokens := map[string]string{}
	for _, h := range listed {
		tokens[h.ID] = h.PingToken
	}
	assert.Equal(t, map[string]string{a.ID: a.PingToken, b.ID: b.PingToken}, tokens)
}

func TestHeartbeatStore_RotatePingTokenKeepsIDAndHistoryAndRefusesACollision(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	hs := NewHeartbeatStore(db)

	a := newStoredHeartbeat("nightly backup", "tok-a-"+uid.New())
	b := newStoredHeartbeat("hourly sync", "tok-b-"+uid.New())
	for _, h := range []*heartbeat.Heartbeat{a, b} {
		_, err := hs.CreateHeartbeat(ctx, h)
		require.NoError(t, err)
	}
	_, err := hs.InsertPing(ctx, &heartbeat.HeartbeatPing{HeartbeatID: a.ID, PingType: heartbeat.PingSuccess, SourceIP: "10.0.0.1", HTTPMethod: "GET", Timestamp: time.Now()})
	require.NoError(t, err)

	_, err = hs.CreateHeartbeat(ctx, newStoredHeartbeat("dup", a.PingToken))
	require.Error(t, err, "two heartbeats never share a token")
	assert.True(t, IsUniqueViolation(err), "a shared token is a unique violation, got %v", err)

	err = hs.RotatePingToken(ctx, a.ID, b.PingToken)
	require.Error(t, err, "a rotation onto another heartbeat's token fails")
	assert.True(t, IsUniqueViolation(err), "got %v", err)
	still, err := hs.GetHeartbeatByPingToken(ctx, a.PingToken)
	require.NoError(t, err)
	require.NotNil(t, still, "the current token stays valid after a failed rotation")
	assert.Equal(t, a.ID, still.ID)

	fresh := "tok-c-" + uid.New()
	require.NoError(t, hs.RotatePingToken(ctx, a.ID, fresh))
	old, err := hs.GetHeartbeatByPingToken(ctx, a.PingToken)
	require.NoError(t, err)
	assert.Nil(t, old, "the old token is unknown at once")
	rotated, err := hs.GetHeartbeatByPingToken(ctx, fresh)
	require.NoError(t, err)
	require.NotNil(t, rotated)
	assert.Equal(t, a.ID, rotated.ID)
	assert.Equal(t, fresh, rotated.PingToken)
	assert.Equal(t, "nightly backup", rotated.Name)

	pings, _, err := hs.ListPings(ctx, a.ID, heartbeat.ListPingsOpts{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, pings, 1, "the ping history follows the id, not the token")

	other, err := hs.GetHeartbeatByPingToken(ctx, b.PingToken)
	require.NoError(t, err)
	require.NotNil(t, other, "the other heartbeat is not affected")
	assert.Equal(t, b.ID, other.ID)

	err = hs.RotatePingToken(ctx, uid.New(), "tok-d-"+uid.New())
	assert.ErrorIs(t, err, heartbeat.ErrHeartbeatNotFound)
}

// migrateTo steps a migrated db back to the given embedded migration through its down files, as a release of that day left it.
// Migrating up from an empty file would not do: the SQLite conversions apply uuid_schema.sql, which is kept at the head.
func migrateTo(t *testing.T, db *DB, version uint) {
	t.Helper()
	source, err := iofs.New(migrationFS, "migrations/"+db.dialect.String())
	require.NoError(t, err)
	var driver database.Driver
	switch db.dialect {
	case DialectPostgres:
		driver, err = migratepgx.WithInstance(db.db, &migratepgx.Config{})
	default:
		driver, err = migratesqlite.WithInstance(db.db, &migratesqlite.Config{})
	}
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", source, db.dialect.String(), driver)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(version))
}

// FR-011, US2 scenario 4, SC-002: a database written before the upgrade keeps every deployed ping URL working,
// on SQLite as on PostgreSQL, because the migration gives each existing heartbeat its id as token.
func TestMigration_ExistingHeartbeatsGetTheirIDAsPingToken(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	migrateTo(t, db, schemaBeforePingToken)

	now := time.Now().Unix()
	ids := []string{uid.New(), uid.New(), uid.New()}
	for i, id := range ids {
		_, err := db.db.ExecContext(ctx, db.dialect.Rebind(`INSERT INTO heartbeats (id, agent_id, name, status, alert_state,
			interval_seconds, grace_seconds, consecutive_failures, consecutive_successes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`),
			id, uid.LocalAgent, "job "+string(rune('a'+i)), string(heartbeat.StatusUp), string(heartbeat.AlertNormal), 300, 60, now, now)
		require.NoError(t, err, "a release at migration %d stores the heartbeat without a token column", schemaBeforePingToken)
	}

	componentID := uid.New()
	_, err := db.db.ExecContext(ctx, db.dialect.Rebind(`INSERT INTO status_components (id, composition_mode, match_all_type, display_name,
		display_order, visible, status_override, override_before_maintenance, auto_incident, created_at, updated_at)
		VALUES (?, 'explicit', NULL, 'API', 0, 1, NULL, NULL, 0, ?, ?)`), componentID, now, now)
	require.NoError(t, err)
	_, err = db.db.ExecContext(ctx, db.dialect.Rebind(`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, 'heartbeat', ?)`), componentID, ids[0])
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, db, testLogger()), "the upgrade needs no manual step")
	hs := NewHeartbeatStore(db)

	comps, err := NewStatusComponentStore(db).ListComponents(ctx)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	require.Len(t, comps[0].Monitors, 1)
	assert.Equal(t, ids[0], comps[0].Monitors[0].ID, "an existing component keeps its monitor")
	assert.Empty(t, comps[0].Monitors[0].Label, "an existing component has no public label")

	for _, id := range ids {
		hb, err := hs.GetHeartbeatByPingToken(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, hb, "/ping/%s deployed before the upgrade still resolves", id)
		assert.Equal(t, id, hb.ID)
		assert.Equal(t, id, hb.PingToken, "the token of an existing heartbeat is its old id")
	}

	_, err = hs.CreateHeartbeat(ctx, newStoredHeartbeat("dup", ids[0]))
	require.Error(t, err, "the migrated tokens are unique")
	assert.True(t, IsUniqueViolation(err), "got %v", err)

	fresh := newStoredHeartbeat("after upgrade", uid.New())
	_, err = hs.CreateHeartbeat(ctx, fresh)
	require.NoError(t, err)
	byToken, err := hs.GetHeartbeatByPingToken(ctx, fresh.PingToken)
	require.NoError(t, err)
	require.NotNil(t, byToken)
	assert.Equal(t, fresh.ID, byToken.ID)
	byID, err := hs.GetHeartbeatByPingToken(ctx, fresh.ID)
	require.NoError(t, err)
	assert.Nil(t, byID, "a heartbeat created after the upgrade is not reachable by its id")
}

// FR-011: the schema moves on both engines under the same number (the equivalence test checks the content).
func TestEmbeddedHeads_MoveBeyondThePingTokenRelease(t *testing.T) {
	sqliteHead, err := EmbeddedHeadVersion(DialectSQLite)
	require.NoError(t, err)
	pgHead, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	assert.Equal(t, sqliteHead, pgHead)
	assert.Greater(t, sqliteHead, uint(schemaBeforePingToken), "the ping token and the public label need a migration on both sides")
}
