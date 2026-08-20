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

package main

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

const copyTestDatabaseURLEnv = "MAINTENANT_TEST_DATABASE_URL"

// sourceInstall creates a migrated local install to copy from.
func sourceInstall(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "maintenant.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := store.Open(path, logger)
	require.NoError(t, err)
	require.NoError(t, store.Migrate(context.Background(), db, logger))
	require.NoError(t, db.Close())
	return path
}

// emptyTargetDSN creates an empty PostgreSQL database, or skips.
func emptyTargetDSN(t *testing.T) string {
	t.Helper()
	adminDSN := os.Getenv(copyTestDatabaseURLEnv)
	if adminDSN == "" {
		t.Skip("PostgreSQL test database not configured (" + copyTestDatabaseURLEnv + ")")
	}

	name := "t_" + strings.ReplaceAll(uid.New(), "-", "")
	admin, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err)
	_, err = admin.ExecContext(context.Background(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		_ = admin.Close()
	})

	u, err := store.ParseDSN(adminDSN)
	require.NoError(t, err)
	u.Path = "/" + name
	return u.String()
}

func runCopyForTest(t *testing.T, dbPath, dsn string, assumeYes bool, stdin string) (int, string, string) {
	t.Helper()
	var out, logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	code := runCopy(dbPath, dsn, assumeYes, &out, strings.NewReader(stdin), logger)
	return code, out.String(), logs.String()
}

// TestRunCopy_AnnouncesThenCopies is the contract's happy path: the
// announcement comes before the write, the counts are verified, and the exit
// code says so.
func TestRunCopy_AnnouncesThenCopies(t *testing.T) {
	dsn := emptyTargetDSN(t)
	code, out, _ := runCopyForTest(t, sourceInstall(t), dsn, true, "")

	assert.Equal(t, copyExitOK, code)

	// What travels, what does not, and the two consequences — before the
	// "Copied" summary, which is the proof it was said before writing.
	announcement := out[:strings.Index(out, "Copied ")]
	assert.Contains(t, announcement, "agents")
	assert.Contains(t, announcement, "enrollment_tokens")
	assert.Contains(t, announcement, "Leaving behind")
	assert.Contains(t, announcement, "resource_snapshots")
	assert.Contains(t, announcement, "unacknowledged")
	assert.Contains(t, announcement, "Curves start from zero")

	assert.Contains(t, out, "counts verified")
	assert.Contains(t, out, "MAINTENANT_DATABASE_URL")
	assert.Contains(t, out, "nothing to do on the monitored machines")
}

// TestRunCopy_AsksBeforeWriting covers the interactive path: no --yes means a
// prompt, and declining writes nothing.
func TestRunCopy_AsksBeforeWriting(t *testing.T) {
	dsn := emptyTargetDSN(t)

	t.Run("declining writes nothing", func(t *testing.T) {
		code, out, _ := runCopyForTest(t, sourceInstall(t), dsn, false, "n\n")
		assert.Equal(t, copyExitRefused, code)
		assert.Contains(t, out, "Proceed?")
		assert.Contains(t, out, "Nothing was written.")

		db, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		var n int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.tables
			WHERE table_schema = current_schema()`).Scan(&n))
		assert.Zero(t, n)
	})

	t.Run("accepting copies", func(t *testing.T) {
		code, out, _ := runCopyForTest(t, sourceInstall(t), dsn, false, "y\n")
		assert.Equal(t, copyExitOK, code)
		assert.Contains(t, out, "counts verified")
	})
}

// TestRunCopy_RefusesNonEmptyTarget: the second copy onto the same database is
// refused, because the first one filled it.
func TestRunCopy_RefusesNonEmptyTarget(t *testing.T) {
	dsn := emptyTargetDSN(t)
	code, _, _ := runCopyForTest(t, sourceInstall(t), dsn, true, "")
	require.Equal(t, copyExitOK, code)

	code, _, logs := runCopyForTest(t, sourceInstall(t), dsn, true, "")
	assert.Equal(t, copyExitRefused, code)
	assert.Contains(t, logs, "not empty")
	assert.Contains(t, logs, "fix=")
}

// TestRunCopy_UnreadableSource fails before touching the target, and says what
// to check.
func TestRunCopy_UnreadableSource(t *testing.T) {
	dsn := emptyTargetDSN(t)
	code, _, logs := runCopyForTest(t, filepath.Join(t.TempDir(), "does-not-exist.db"), dsn, true, "")

	assert.Equal(t, copyExitRefused, code)
	assert.Contains(t, logs, "source")
}

// TestRunCopy_InvalidTargetDSN never opens anything and never echoes the
// credential.
func TestRunCopy_InvalidTargetDSN(t *testing.T) {
	const password = "s3cr3t-Sentinel-copy"
	code, _, logs := runCopyForTest(t, sourceInstall(t), "mysql://app:"+password+"@db/x", true, "")

	assert.Equal(t, copyExitRefused, code)
	assert.NotContains(t, logs, password)
}
