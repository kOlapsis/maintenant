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

// Package storetest opens migrated test databases for the packages whose
// suites exercise the store: SQLite by default, PostgreSQL when
// MAINTENANT_TEST_DATABASE_URL points at an admin role, so the same business
// assertions run on both engines (SC-002). It is only ever imported from
// _test files.
package storetest

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

// EnvVar selects the engine: unset means SQLite; an admin DSN (a role that can
// CREATE DATABASE) runs the suite on PostgreSQL, one fresh database per test.
const EnvVar = "MAINTENANT_TEST_DATABASE_URL"

// RequireEnvVar turns every PostgreSQL skip into a failure. CI sets it on the
// steps that are supposed to exercise PostgreSQL, so a typo in EnvVar, a
// service that never came up, or a step that silently lost its environment
// fails loudly instead of passing green having tested nothing.
//
// Without it, a suite whose PostgreSQL cases all skip reports "ok" — which is
// exactly the failure mode this guard exists to make impossible.
const RequireEnvVar = "MAINTENANT_REQUIRE_POSTGRES"

// AdminDSN returns the configured admin DSN. It skips the test when none is
// configured, unless RequireEnvVar says PostgreSQL coverage was promised, in
// which case its absence is the bug.
func AdminDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv(EnvVar)
	if dsn != "" {
		return dsn
	}
	if os.Getenv(RequireEnvVar) != "" {
		t.Fatalf("%s is set, so this run must exercise PostgreSQL, but %s is empty: "+
			"the coverage it promises would silently not happen", RequireEnvVar, EnvVar)
	}
	t.Skip("PostgreSQL test database not configured (" + EnvVar + ")")
	return ""
}

// Open returns a migrated *store.DB with its writer running, torn down with
// the test. The engine follows EnvVar.
func Open(t *testing.T, logger *slog.Logger) *store.DB {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	if adminDSN := os.Getenv(EnvVar); adminDSN != "" {
		return openPostgres(t, adminDSN, logger)
	}
	if os.Getenv(RequireEnvVar) != "" {
		t.Fatalf("%s is set, so this run must exercise PostgreSQL, but %s is empty: "+
			"the coverage it promises would silently not happen", RequireEnvVar, EnvVar)
	}
	return openSQLite(t, logger)
}

func openSQLite(t *testing.T, logger *slog.Logger) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	migrateAndStart(t, db, logger)
	return db
}

func openPostgres(t *testing.T, adminDSN string, logger *slog.Logger) *store.DB {
	t.Helper()
	ctx := context.Background()

	name := "t_" + strings.ReplaceAll(uid.New(), "-", "")
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		_ = admin.Close()
	})

	u, err := store.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("parse admin DSN: %v", err)
	}
	u.Path = "/" + name
	db, err := store.OpenPostgres(ctx, u.String(), logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	migrateAndStart(t, db, logger)
	return db
}

func migrateAndStart(t *testing.T, db *store.DB, logger *slog.Logger) {
	t.Helper()
	if err := store.Migrate(context.Background(), db, logger); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)
	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
}
