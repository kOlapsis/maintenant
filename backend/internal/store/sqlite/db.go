package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite database connection with PulseBoard configuration.
type DB struct {
	db     *sql.DB
	writer *Writer
	logger *slog.Logger
}

// Open creates and configures a SQLite database connection with WAL mode.
func Open(dbPath string, logger *slog.Logger) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-8000&_foreign_keys=ON", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Set connection pool to 1 for writes (serialized via Writer),
	// but allow multiple read connections.
	db.SetMaxOpenConns(4)

	// Apply PRAGMAs that can't be set via DSN
	pragmas := []string{
		"PRAGMA auto_vacuum = INCREMENTAL",
		"PRAGMA wal_autocheckpoint = 1000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec pragma %q: %w", p, err)
		}
	}

	sdb := &DB{
		db:     db,
		logger: logger,
	}

	return sdb, nil
}

// StartWriter initializes and starts the single-writer goroutine.
func (d *DB) StartWriter(ctx context.Context) {
	d.writer = NewWriter(d.db, d.logger)
	d.writer.Start(ctx)
}

// Writer returns the serialized write channel.
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
