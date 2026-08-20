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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/require"
)

// testDatabaseURLEnv selects the engine the suite runs against. Unset: SQLite,
// as always. Set to an admin DSN (a role that can CREATE DATABASE): the same
// suite runs on PostgreSQL, one fresh database per test (SC-002). PostgreSQL
// cases skip, never fail, when it is unset (R11).
const testDatabaseURLEnv = "MAINTENANT_TEST_DATABASE_URL"

// openTestDB creates a temporary database with all migrations applied and the
// writer running for the duration of the test. The engine is selected by
// MAINTENANT_TEST_DATABASE_URL.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	if adminDSN := os.Getenv(testDatabaseURLEnv); adminDSN != "" {
		return openTestPostgres(t, adminDSN)
	}
	return openTestSQLite(t)
}

func openTestSQLite(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := testLogger()

	db, err := Open(dbPath, logger)
	require.NoError(t, err, "open test DB")

	err = Migrate(context.Background(), db, logger)
	require.NoError(t, err, "run migrations on test DB")

	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)

	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
	return db
}

func openTestPostgres(t *testing.T, adminDSN string) *DB {
	t.Helper()
	logger := testLogger()
	ctx := context.Background()

	dsn := createTestDatabase(t, adminDSN)
	db, err := OpenPostgres(ctx, dsn, logger)
	require.NoError(t, err, "open test database")

	err = Migrate(ctx, db, logger)
	require.NoError(t, err, "run migrations on test database")

	wctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(wctx)

	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
	return db
}

// createTestDatabase creates a fresh, empty database on the configured test
// server and returns its DSN. It is dropped when the test ends.
func createTestDatabase(t *testing.T, adminDSN string) string {
	t.Helper()
	ctx := context.Background()

	name := "t_" + strings.ReplaceAll(uid.New(), "-", "")
	admin, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err, "open admin connection")
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err, "create test database")

	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		_ = admin.Close()
	})

	u, err := ParseDSN(adminDSN)
	require.NoError(t, err)
	u.Path = "/" + name
	return u.String()
}

// requireSQLite skips tests that exercise the local file itself: PRAGMAs, the
// historical conversions, WAL maintenance, rowid forms.
func requireSQLite(t *testing.T) {
	t.Helper()
	if os.Getenv(testDatabaseURLEnv) != "" {
		t.Skip("SQLite-specific test, suite is running on PostgreSQL")
	}
}

// requirePostgres returns a fresh PostgreSQL *DB or skips when no test
// database is configured (R11: skipped, never failed).
func requirePostgres(t *testing.T) *DB {
	t.Helper()
	adminDSN := os.Getenv(testDatabaseURLEnv)
	if adminDSN == "" {
		t.Skip("PostgreSQL test database not configured (" + testDatabaseURLEnv + ")")
	}
	return openTestPostgres(t, adminDSN)
}

// testAdminDSN returns the configured admin DSN or skips.
func testAdminDSN(t *testing.T) string {
	t.Helper()
	adminDSN := os.Getenv(testDatabaseURLEnv)
	if adminDSN == "" {
		t.Skip("PostgreSQL test database not configured (" + testDatabaseURLEnv + ")")
	}
	return adminDSN
}
