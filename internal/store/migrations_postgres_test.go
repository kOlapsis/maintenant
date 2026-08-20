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
// starting together on an empty database stay correct — exactly one installs
// (the pgx driver serializes appliers with pg_advisory_lock).
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

	// Rewind to the baseline: drop what migration 29 created and set the
	// recorded version back.
	_, err = db.ReadDB().Exec("DROP TABLE instances")
	require.NoError(t, err)
	_, err = db.ReadDB().Exec("UPDATE schema_migrations SET version = 28, dirty = false")
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
	v, err := db.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint(29), v)
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
