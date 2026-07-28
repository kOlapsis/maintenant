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
	"fmt"
	"log/slog"

	"github.com/mattn/go-sqlite3"
)

// driverName registers our own driver so every pooled connection gets the
// per-connection PRAGMAs below. They are not persisted in the database file and
// the go-sqlite3 DSN parser does not accept them, so a ConnectHook is the only
// way to apply them to connections the pool opens later.
const driverName = "sqlite3_maintenant"

func init() {
	// These bound the WAL. Without journal_size_limit the -wal file grows
	// without limit on a continuously written database and is only truncated
	// when the process restarts.
	pragmas := []string{
		"PRAGMA wal_autocheckpoint = 1000",     // pages, ~4 MB
		"PRAGMA journal_size_limit = 67108864", // 64 MiB
	}
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			for _, p := range pragmas {
				if _, err := conn.Exec(p, nil); err != nil {
					return fmt.Errorf("exec pragma %q: %w", p, err)
				}
			}
			return nil
		},
	})
}

// DB wraps a SQLite database connection with maintenant configuration.
type DB struct {
	db     *sql.DB
	writer *Writer
	logger *slog.Logger

	// incrementalVacuum records whether the file header allows reclaiming pages
	// without a full VACUUM. Databases created before auto_vacuum was set stay
	// in NONE mode for good, and incremental_vacuum is a no-op on them.
	incrementalVacuum bool
}

// Open creates and configures a SQLite database connection with WAL mode.
//
// This is the only SQLite-specific seam: the schema (uuid_schema.sql) and all
// queries use the portable common subset (TEXT/BIGINT/INTEGER/REAL/BLOB ids and
// timestamps, ON CONFLICT upserts, `?` placeholders, no AUTOINCREMENT/PRAGMA).
// A Postgres backend is a drop-in: its own Open (pgx DSN), a `?`→`$n` placeholder
// rewrite, and a Postgres migrations dir — no schema or query changes.
func Open(dbPath string, logger *slog.Logger) (*DB, error) {
	// auto_vacuum has to be part of the DSN, not a later Exec: switching to WAL
	// writes the file header, and once that is done the pragma is silently
	// ignored. The driver applies the DSN pragmas in the working order.
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_auto_vacuum=incremental&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-8000&_foreign_keys=ON", dbPath)

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Set a connection pool to 1 for writes (serialized via Writer),
	// but allow multiple read connections.
	db.SetMaxOpenConns(4)

	sdb := &DB{
		db:     db,
		writer: NewWriter(db, logger),
		logger: logger,
	}
	sdb.readStorageMode()

	return sdb, nil
}

// readStorageMode reports the settings that decide whether freed pages ever
// return to the filesystem, and records the answer for the retention cleanup.
func (d *DB) readStorageMode() {
	var autoVacuum, pageSize, freelist int
	row := d.db.QueryRow("PRAGMA auto_vacuum")
	if err := row.Scan(&autoVacuum); err != nil {
		d.logger.Warn("sqlite: cannot read storage mode", "error", err)
		return
	}
	_ = d.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	_ = d.db.QueryRow("PRAGMA freelist_count").Scan(&freelist)

	modes := map[int]string{0: "none", 1: "full", 2: "incremental"}
	mode, ok := modes[autoVacuum]
	if !ok {
		mode = fmt.Sprintf("unknown(%d)", autoVacuum)
	}
	d.incrementalVacuum = autoVacuum == 2

	d.logger.Info("sqlite storage",
		"auto_vacuum", mode,
		"page_size", pageSize,
		"freelist_pages", freelist)
	if autoVacuum == 0 {
		d.logger.Warn("sqlite: auto_vacuum is NONE, space freed by retention stays inside the file; run a manual VACUUM to return it to the filesystem",
			"reclaimable_pages", freelist)
	}
}

// StartWriter starts the single-writer goroutine.
func (d *DB) StartWriter(ctx context.Context) {
	d.writer.Start(ctx)
}

// Writer returns the serialized to write channel.
func (d *DB) Writer() *Writer {
	return d.writer
}

// ReadDB returns the underlying sql.DB for read operations.
func (d *DB) ReadDB() *sql.DB {
	return d.db
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}
