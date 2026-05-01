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
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// openTestDB creates a temporary SQLite database with all migrations applied
// and the writer goroutine running for the duration of the test.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(dbPath, logger)
	require.NoError(t, err, "open test DB")

	err = Migrate(db.ReadDB(), logger)
	require.NoError(t, err, "run migrations on test DB")

	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)

	t.Cleanup(func() {
		cancel()
		_ = db.Close()
	})
	return db
}
