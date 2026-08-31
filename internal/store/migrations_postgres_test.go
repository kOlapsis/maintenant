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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigratePostgres_FreshDatabase pins FR-014: an empty database gets the
// whole schema without any manual step, and a second run finds nothing to do.
func TestMigratePostgres_FreshDatabase(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, Migrate(ctx, db, logger))

	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	v, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, v, "fresh database must land on the embedded head")

	// The sentinel agent is installed by the baseline.
	var hostname string
	require.NoError(t, db.ReadDB().QueryRow(
		"SELECT hostname FROM agents WHERE id = '00000000-0000-0000-0000-000000000000'").Scan(&hostname))
	assert.Equal(t, "local", hostname)

	// Second run: no pending migrations, no error, same version.
	require.NoError(t, Migrate(ctx, db, logger))
	v2, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, v, v2)
}

// TestMigratePostgres_ConcurrentInstall pins FR-016: several instances
// starting together on an empty database stay correct, exactly one installs
// and the others wait on the migration lock and find nothing to do.
func TestMigratePostgres_ConcurrentInstall(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	const n = 4
	dbs := make([]*DB, n)
	for i := range dbs {
		db, err := OpenPostgres(ctx, dsn, logger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		dbs[i] = db
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i, db := range dbs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Migrate(ctx, db, logger)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent migrator %d", i)
	}

	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	v, err := dbs[0].SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, v)

	// Every table exists exactly once, the sentinel row too.
	var agents int
	require.NoError(t, dbs[0].ReadDB().QueryRow("SELECT count(*) FROM agents").Scan(&agents))
	assert.Equal(t, 1, agents, "exactly one sentinel agent")
}

// TestMigratePostgres_ConcurrentCatchUp pins FR-016's "or behind" half: a
// database sitting at the baseline is brought to the head by several
// concurrent instances without error.
func TestMigratePostgres_ConcurrentCatchUp(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(ctx, db, logger))

	// Rewind to the baseline: undo every migration written above it, then set
	// the recorded version back. Each new migration adds a line here — that is
	// the price of a test that catches up over more than one step.
	for _, undo := range []string{
		"DROP TABLE instances", // 29
		"ALTER TABLE notification_channels DROP COLUMN secret, DROP COLUMN config", // 30
	} {
		_, err = db.ReadDB().Exec(undo)
		require.NoError(t, err, undo)
	}
	_, err = db.ReadDB().Exec("UPDATE schema_migrations SET version = $1, dirty = false", postgresBaselineVersion)
	require.NoError(t, err)

	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		mdb, err := OpenPostgres(ctx, dsn, logger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = mdb.Close() })
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Migrate(ctx, mdb, logger)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent catch-up %d", i)
	}
	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	v, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, v)
	var one int
	require.NoError(t, db.ReadDB().QueryRow("SELECT count(*) FROM instances").Scan(&one))
	assert.Zero(t, one, "instances recreated empty")
}

// TestMigratePostgres_RefusesNewerSchema pins FR-017 on PostgreSQL.
func TestMigratePostgres_RefusesNewerSchema(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(ctx, db, logger))

	_, err = db.ReadDB().Exec("UPDATE schema_migrations SET version = 999")
	require.NoError(t, err)

	err = Migrate(ctx, db, logger)
	require.ErrorIs(t, err, ErrSchemaNewer)
	assert.Contains(t, err.Error(), "999")
}

// TestMigratePostgres_DirtyBaselineRecovers pins the recovery path on the
// engine whose history starts at the baseline: an install interrupted halfway
// through migration 28 must be recovered to the empty database, not to a
// version 28 - 1 that no file describes.
func TestMigratePostgres_DirtyBaselineRecovers(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// An interrupted baseline: the version is recorded dirty, and the
	// transaction that carried the schema rolled back.
	_, err = db.ReadDB().Exec(`CREATE TABLE schema_migrations (
		version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL)`)
	require.NoError(t, err)
	_, err = db.ReadDB().Exec("INSERT INTO schema_migrations (version, dirty) VALUES (28, true)")
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, db, logger))

	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	v, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, v, "the interrupted install must be replayed to the head")

	var agents int
	require.NoError(t, db.ReadDB().QueryRow("SELECT count(*) FROM agents").Scan(&agents))
	assert.Equal(t, 1, agents, "the replayed baseline installs the sentinel agent")
}

// TestMigratePostgres_WaitsForTheInstanceMigrating pins the other half of
// FR-016: the version read and the dirty recovery are inside the lock, not
// only the apply. An instance starting while another migrates must wait for it
// rather than read its in-flight state.
func TestMigratePostgres_WaitsForTheInstanceMigrating(t *testing.T) {
	dsn := createTestDatabase(t, testAdminDSN(t))
	ctx := context.Background()
	logger := testLogger()

	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Hold the lock the way an instance in the middle of installing does.
	holder, err := db.ReadDB().Conn(ctx)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- Migrate(ctx, db, logger) }()

	select {
	case err := <-done:
		require.Fail(t, "Migrate ran while another instance held the lock", "returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	_, err = holder.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey)
	require.NoError(t, err)
	require.NoError(t, holder.Close())

	require.NoError(t, <-done)
	head, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	v, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, v)
}
