package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate runs all pending database migrations using golang-migrate v4.
// On first run after upgrading from the custom migration system, it bootstraps
// the schema_migrations table from the old schema_version table.
func Migrate(db *sql.DB, logger *slog.Logger) error {
	// Bootstrap from custom schema_version if upgrading from old system
	if err := bootstrapFromCustomSchema(db, logger); err != nil {
		return fmt.Errorf("bootstrap from custom schema: %w", err)
	}

	// Create iofs source from embedded migration files
	source, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	// Create sqlite3 database driver from existing connection
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{
		NoTxWrap: false,
	})
	if err != nil {
		return fmt.Errorf("create sqlite3 driver: %w", err)
	}

	// Create migrator
	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	// Log current version before applying
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("get current version: %w", err)
	}
	if err == migrate.ErrNilVersion {
		logger.Info("database migration", "current_version", 0, "status", "fresh database")
	} else {
		logger.Info("database migration", "current_version", version, "dirty", dirty)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state at version %d — manual intervention required", version)
	}

	// Apply all pending migrations
	err = m.Up()
	if err == migrate.ErrNoChange {
		logger.Info("database migration", "status", "no pending migrations")
		return nil
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	// Log the new version after applying
	newVersion, _, _ := m.Version()
	logger.Info("database migration", "status", "migrations applied", "new_version", newVersion)

	return nil
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

	if err == sql.ErrNoRows {
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
		// Old table exists but is empty — just drop it
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
