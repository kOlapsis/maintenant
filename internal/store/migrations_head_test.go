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
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postgresBaselineVersion is the migration number of the PostgreSQL baseline:
// the SQLite head on the day the baseline was written. Everything above it is
// written for both engines under the same number.
const postgresBaselineVersion = 28

func embeddedNumbers(t *testing.T, dialect Dialect) map[uint]bool {
	t.Helper()
	entries, err := fs.ReadDir(migrationFS, "migrations/"+dialect.String())
	require.NoError(t, err)
	nums := map[uint]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		n, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 32)
		require.NoError(t, err, name)
		nums[uint(n)] = true
	}
	return nums
}

// TestEmbeddedHeads_SameVersion is what makes the shared-migration rule
// tenable: any migration >= 29 exists on both sides under the same number, so
// the two heads can never silently diverge.
func TestEmbeddedHeads_SameVersion(t *testing.T) {
	sqliteHead, err := EmbeddedHeadVersion(DialectSQLite)
	require.NoError(t, err)
	pgHead, err := EmbeddedHeadVersion(DialectPostgres)
	require.NoError(t, err)
	assert.Equal(t, sqliteHead, pgHead, "the two embedded migration heads must carry the same number")

	sqliteNums := embeddedNumbers(t, DialectSQLite)
	pgNums := embeddedNumbers(t, DialectPostgres)

	// The PostgreSQL source is the baseline plus the shared suffix: nothing
	// below the baseline number.
	for n := range pgNums {
		assert.GreaterOrEqual(t, n, uint(postgresBaselineVersion),
			"postgres source carries a pre-baseline migration %d", n)
	}

	// Every shared-era migration exists on both sides.
	for n := uint(postgresBaselineVersion) + 1; n <= sqliteHead; n++ {
		assert.True(t, sqliteNums[n], "migration %d missing on the sqlite side", n)
		assert.True(t, pgNums[n], "migration %d missing on the postgres side", n)
	}
	assert.True(t, pgNums[postgresBaselineVersion], "postgres baseline missing")
}

// TestMigrate_RefusesNewerSchema pins FR-017 on SQLite: a binary older than
// the schema refuses to start instead of writing into it.
func TestMigrate_RefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(filepath.Join(dir, "newer.db"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	require.NoError(t, Migrate(ctx, db, logger))

	// Simulate a schema produced by a future release.
	_, err = db.ReadDB().Exec("UPDATE schema_migrations SET version = 999")
	require.NoError(t, err)

	err = Migrate(ctx, db, logger)
	require.ErrorIs(t, err, ErrSchemaNewer)
	assert.Contains(t, err.Error(), "999")

	// Nothing was written: the version is still the future one.
	var v int
	require.NoError(t, db.ReadDB().QueryRow("SELECT version FROM schema_migrations").Scan(&v))
	assert.Equal(t, 999, v)
}
