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

package agent

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var spoolMigrationFS embed.FS

const (
	spoolFile              = "spool.db"
	metaDroppedSinceConn   = "dropped_since_connect"
	spoolVacuumPagesPerRun = 2000
	spoolDropScanLimit     = 50000
)

// spoolStore is the SQLite side of the spool: durable storage, ordered reads
// and bounded purges.
type spoolStore struct {
	db   *sql.DB
	path string
}

// openSpoolStore opens (creating it if needed) the agent spool database and
// brings it to the embedded schema head.
func openSpoolStore(dataDir string) (*spoolStore, error) {
	path := filepath.Join(dataDir, spoolFile)

	// auto_vacuum belongs in the DSN, not a later Exec: switching to WAL writes
	// the file header, and the pragma is silently ignored afterwards.
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_auto_vacuum=incremental&_busy_timeout=5000&_synchronous=NORMAL", path)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open spool database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open spool database %s: %w", path, err)
	}

	if err := migrateSpool(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &spoolStore{db: db, path: path}, nil
}

func migrateSpool(db *sql.DB) error {
	source, err := iofs.New(spoolMigrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("create spool migration source: %w", err)
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{NoTxWrap: false})
	if err != nil {
		return fmt.Errorf("create spool migrate driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("create spool migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate spool database: %w", err)
	}
	return nil
}

func (s *spoolStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close spool database: %w", err)
	}
	return nil
}

// spooledEvent is one row of spool_events, payload still serialized.
type spooledEvent struct {
	Seq     int64
	Payload []byte
}

// Append writes a batch of events in one transaction and returns the sequence
// assigned to the last one.
func (s *spoolStore) Append(events []pendingEvent) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin spool append: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO spool_events(event_id, observed_at, payload, size_bytes) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare spool insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	var last int64
	for _, ev := range events {
		res, err := stmt.Exec(ev.EventID, ev.ObservedAt, ev.Payload, len(ev.Payload))
		if err != nil {
			return 0, fmt.Errorf("insert spool event: %w", err)
		}
		if last, err = res.LastInsertId(); err != nil {
			return 0, fmt.Errorf("read spool sequence: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit spool append: %w", err)
	}
	return last, nil
}

// ReadFrom returns up to limit events with a sequence strictly above after,
// oldest first.
func (s *spoolStore) ReadFrom(after int64, limit int) ([]spooledEvent, error) {
	rows, err := s.db.Query(
		`SELECT seq, payload FROM spool_events WHERE seq > ? ORDER BY seq LIMIT ?`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("read spool events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []spooledEvent
	for rows.Next() {
		var ev spooledEvent
		if err := rows.Scan(&ev.Seq, &ev.Payload); err != nil {
			return nil, fmt.Errorf("scan spool event: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read spool events: %w", err)
	}
	return out, nil
}

// DeleteUpTo removes every event whose sequence is at or below seq.
func (s *spoolStore) DeleteUpTo(seq int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM spool_events WHERE seq <= ?`, seq)
	if err != nil {
		return 0, fmt.Errorf("purge acknowledged spool events: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged spool events: %w", err)
	}
	if n > 0 {
		if err := s.reclaim(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// DeleteOlderThan removes every event observed before cutoff.
func (s *spoolStore) DeleteOlderThan(cutoff int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM spool_events WHERE observed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge expired spool events: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count expired spool events: %w", err)
	}
	if n > 0 {
		if err := s.reclaim(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// DeleteOldestBytes removes the oldest events until at least target bytes of
// payload have been dropped, and returns how many rows went. The scan is capped
// so one call stays cheap on a large spool; the caller comes back if needed.
func (s *spoolStore) DeleteOldestBytes(target int64) (int64, error) {
	res, err := s.db.Exec(`
		DELETE FROM spool_events WHERE seq <= (
			SELECT MAX(seq) FROM (
				SELECT seq, size_bytes, SUM(size_bytes) OVER (ORDER BY seq) AS running
				FROM spool_events ORDER BY seq LIMIT ?
			) WHERE running - size_bytes < ?
		)`, spoolDropScanLimit, target)
	if err != nil {
		return 0, fmt.Errorf("drop oldest spool events: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count dropped spool events: %w", err)
	}
	if n > 0 {
		if err := s.reclaim(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// reclaim returns pages freed by a delete to the filesystem. Without it a
// drained spool keeps the size of its worst outage.
//
// It has to run through Query and drain the result: incremental_vacuum frees
// one page per step, and Exec only ever takes one.
func (s *spoolStore) reclaim() error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", spoolVacuumPagesPerRun))
	if err != nil {
		return fmt.Errorf("reclaim spool pages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() { //nolint:revive // stepping is the work; the pragma yields no rows
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reclaim spool pages: %w", err)
	}
	return nil
}

// SizeBytes reports the on-disk size of the spool database.
func (s *spoolStore) SizeBytes() (int64, error) {
	var pageCount, pageSize int64
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("read spool page count: %w", err)
	}
	if err := s.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("read spool page size: %w", err)
	}
	return pageCount * pageSize, nil
}

// Depth reports how many events are still queued.
func (s *spoolStore) Depth() (int64, error) {
	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM spool_events`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count spool events: %w", err)
	}
	return n, nil
}

// MinSeq returns the lowest queued sequence, or 0 when the spool is empty.
func (s *spoolStore) MinSeq() (int64, error) {
	var seq sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(seq) FROM spool_events`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("read lowest spool sequence: %w", err)
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

func (s *spoolStore) meta(key string) (int64, error) {
	var v int64
	if err := s.db.QueryRow(`SELECT value FROM spool_meta WHERE key = ?`, key).Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("read spool meta %s: %w", key, err)
	}
	return v, nil
}

func (s *spoolStore) resetDroppedSinceConnect() error {
	if _, err := s.db.Exec(`UPDATE spool_meta SET value = 0 WHERE key = ?`, metaDroppedSinceConn); err != nil {
		return fmt.Errorf("reset dropped spool counter: %w", err)
	}
	return nil
}

func (s *spoolStore) addDropped(n int64) error {
	if n <= 0 {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO spool_meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = value + excluded.value`,
		metaDroppedSinceConn, n)
	if err != nil {
		return fmt.Errorf("record dropped spool events: %w", err)
	}
	return nil
}
