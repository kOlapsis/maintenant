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
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Two migration sources, one version number. SQLite carries the full history
// (1..N). PostgreSQL starts at a single baseline numbered at the SQLite head
// of the day it was written (28), then shares every migration from 29 onward
// under the same number. A dedicated test compares the two heads and fails on
// divergence: any migration >= 29 is written for both engines or not at all.
//
//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS

// Migrate brings db's schema to the embedded head using golang-migrate v4. On
// SQLite it also runs the historical one-time conversions; PostgreSQL has no
// such past. A schema written by a newer release is refused (FR-017), on both
// engines, rather than written into.
func Migrate(ctx context.Context, db *DB, logger *slog.Logger) error {
	switch db.dialect {
	case DialectSQLite:
		// Bootstrap from the pre-golang-migrate custom schema_version table.
		if err := bootstrapFromCustomSchema(db.db, logger); err != nil {
			return fmt.Errorf("bootstrap from custom schema: %w", err)
		}
	case DialectPostgres:
		// Instances start together against one database (FR-016), so the whole
		// sequence below has to be serialized, not just the apply.
		unlock, err := db.lockMigrations(ctx)
		if err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		defer unlock()
	}

	source, err := iofs.New(migrationFS, "migrations/"+db.dialect.String())
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	var driver database.Driver
	switch db.dialect {
	case DialectPostgres:
		driver, err = migratepgx.WithInstance(db.db, &migratepgx.Config{})
	default:
		driver, err = migratesqlite.WithInstance(db.db, &migratesqlite.Config{NoTxWrap: false})
	}
	if err != nil {
		return fmt.Errorf("create %s migrate driver: %w", db.dialect, err)
	}

	m, err := migrate.NewWithInstance("iofs", source, db.dialect.String(), driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	head, err := EmbeddedHeadVersion(db.dialect)
	if err != nil {
		return fmt.Errorf("read embedded head version: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("get current version: %w", err)
	}
	if errors.Is(err, migrate.ErrNilVersion) {
		logger.Info("database migration", "current_version", 0, "status", "fresh database")
	} else {
		logger.Info("database migration", "current_version", version, "dirty", dirty)
	}

	// FR-017: never write into a schema produced by a newer release. The
	// version numbers are comparable across engines by construction.
	if version > head {
		return fmt.Errorf("%w: schema version %d, this binary knows up to %d",
			ErrSchemaNewer, version, head)
	}

	if dirty {
		// Back to the previous embedded migration, which is not version-1:
		// PostgreSQL's history starts at the baseline, so stepping back by one
		// from it lands on a number no file describes and every later Up()
		// fails on it. No predecessor means back to the empty database.
		target := -1
		if prev, prevErr := source.Prev(version); prevErr == nil {
			target = int(prev)
		}
		logger.Warn("dirty migration state detected, auto-recovering",
			"version", version, "recover_to", target)
		if forceErr := m.Force(target); forceErr != nil {
			return fmt.Errorf("database is in dirty state at version %d and auto-recovery failed: %w", version, forceErr)
		}
	}

	// Apply all pending migrations. Instances that started together are
	// waiting on the lock taken above, so exactly one installs and the others
	// find nothing to do.
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		logger.Info("database migration", "status", "no pending migrations")
	} else {
		newVersion, _, _ := m.Version()
		logger.Info("database migration", "status", "migrations applied", "new_version", newVersion)
	}

	if db.dialect != DialectSQLite {
		return nil
	}

	// Historical one-time conversions, SQLite path only: they describe the
	// local file's past, which no PostgreSQL database has.

	// One-time, in-place conversion of the integer-keyed schema to the
	// UUID-native schema. Guarded by the schema_meta marker it creates.
	if err := convertToUUID(ctx, db.db, logger); err != nil {
		return fmt.Errorf("uuid conversion: %w", err)
	}

	// One-time rebuild extending the cert monitor identity with the SNI
	// server_name, for databases converted before that column existed.
	if err := rebuildCertMonitorsForSNI(ctx, db.db, logger); err != nil {
		return fmt.Errorf("cert sni rebuild: %w", err)
	}

	// One-time rebuild widening the endpoint status CHECK to accept 'degraded',
	// for databases converted before that state existed.
	if err := rebuildEndpointsForDegraded(ctx, db.db, logger); err != nil {
		return fmt.Errorf("endpoint degraded rebuild: %w", err)
	}

	// One-time rebuild replacing the cleartext enrollment token with its hash,
	// for databases converted before the token stopped being stored in clear.
	if err := rebuildEnrollmentTokensForHashing(ctx, db.db, logger); err != nil {
		return fmt.Errorf("enrollment token hash rebuild: %w", err)
	}

	return nil
}

// EmbeddedHeadVersion returns the highest migration number embedded for the
// dialect. It is what FR-017 compares the database's version against.
func EmbeddedHeadVersion(d Dialect) (uint, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations/"+d.String())
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	var head uint64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		n, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("malformed migration name %q: %w", name, err)
		}
		if n > head {
			head = n
		}
	}
	if head == 0 {
		return 0, fmt.Errorf("no embedded migrations for dialect %s", d)
	}
	return uint(head), nil
}

// SchemaVersion reads the applied schema version, for the startup log line and
// the health diagnostic. golang-migrate keeps exactly one row.
func (d *DB) SchemaVersion(ctx context.Context) (uint, error) {
	var v uint
	err := d.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations LIMIT 1").Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}

// bootstrapFromCustomSchema checks if the old custom schema_version table exists
// and migrates its version info to golang-migrate's schema_migrations table.
// This is a one-time operation for existing databases upgrading from the custom system.
func bootstrapFromCustomSchema(db *sql.DB, logger *slog.Logger) error {
	// Check if the old schema_version table exists
	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&tableName)

	if errors.Is(err, sql.ErrNoRows) {
		// No old schema_version table — nothing to bootstrap
		return nil
	}
	if err != nil {
		return fmt.Errorf("check schema_version existence: %w", err)
	}

	// Read the current version from the old system
	var version int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("read custom schema version: %w", err)
	}

	if version == 0 {
		// An old table exists but is empty — just drop it
		if _, err := db.Exec("DROP TABLE schema_version"); err != nil {
			return fmt.Errorf("drop empty schema_version: %w", err)
		}
		logger.Info("bootstrap migration", "action", "dropped empty schema_version table")
		return nil
	}

	logger.Info("bootstrap migration", "action", "migrating from custom schema_version", "old_version", version)

	// Create golang-migrate's schema_migrations table and seed it
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT NOT NULL PRIMARY KEY,
			dirty BOOLEAN NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	if _, err := db.Exec(
		"INSERT OR REPLACE INTO schema_migrations (version, dirty) VALUES (?, ?)",
		version, false,
	); err != nil {
		return fmt.Errorf("seed schema_migrations: %w", err)
	}

	// Drop the old schema_version table
	if _, err := db.Exec("DROP TABLE schema_version"); err != nil {
		return fmt.Errorf("drop schema_version: %w", err)
	}

	logger.Info("bootstrap migration", "action", "completed", "migrated_version", version)
	return nil
}
