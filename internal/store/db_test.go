package store

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterAvailableBeforeStart(t *testing.T) {
	requireSQLite(t)
	// The Writer must be non-nil immediately after Open, before StartWriter is called.
	// Stores capture d.Writer() at construction time (in app.New), but StartWriter
	// is only called later (in app.Start). A nil Writer causes a panic on first write.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	db, err := Open(dbPath, logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	w := db.Writer()
	assert.NotNil(t, w, "Writer() must return a non-nil *Writer before StartWriter is called")
}

// A database created by Open must end up in incremental auto-vacuum mode, which
// is what lets retention hand freed pages back to the filesystem. The mode can
// only be set while the file is still empty, so this has to hold from the start.
func TestOpenEnablesIncrementalAutoVacuum(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)

	var mode int
	require.NoError(t, db.ReadDB().QueryRow("PRAGMA auto_vacuum").Scan(&mode))
	assert.Equal(t, 2, mode, "a fresh database must use incremental auto-vacuum")
	assert.True(t, db.incrementalVacuum, "Open must record the mode for the retention cleanup")
}

// journal_size_limit and wal_autocheckpoint are per-connection settings, so
// running them once through *sql.DB only configures whichever connection the
// pool happened to hand out. An unbounded WAL on the other connections is what
// let the -wal file reach hundreds of megabytes in production.
func TestOpenAppliesPragmasToEveryPooledConnection(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	ctx := context.Background()

	// Hold every connection of the pool at once so each one is distinct.
	conns := make([]*sql.Conn, 0, 4)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	for i := range 4 {
		conn, err := db.ReadDB().Conn(ctx)
		require.NoError(t, err)
		conns = append(conns, conn)

		var sizeLimit, autocheckpoint int64
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_size_limit").Scan(&sizeLimit))
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA wal_autocheckpoint").Scan(&autocheckpoint))

		assert.EqualValues(t, 67108864, sizeLimit, "connection %d has an unbounded WAL", i)
		assert.EqualValues(t, 1000, autocheckpoint, "connection %d has no autocheckpoint", i)
	}
}
