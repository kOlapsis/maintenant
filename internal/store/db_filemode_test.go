// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: the SQLite file is only readable by its owner.

// FR-001, US1 scenario 1: a database created by Maintenant is 0600.
func TestOpen_CreatesTheDatabaseFileAs0600(t *testing.T) {
	requireSQLite(t)
	for _, name := range []string{"maintenant.db", "agent-data.sqlite"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)

			db, err := Open(path, testLogger())
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			require.NoError(t, db.PingContext(context.Background()))

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "the database must be closed to group and others")
		})
	}
}

// FR-001, US1 scenario 1: an existing database keeps the mode the operator gave it.
func TestOpen_KeepsTheModeOfAnExistingDatabase(t *testing.T) {
	requireSQLite(t)
	path := filepath.Join(t.TempDir(), "existing.db")
	first, err := Open(path, testLogger())
	require.NoError(t, err)
	require.NoError(t, first.PingContext(context.Background()))
	require.NoError(t, first.Close())
	require.NoError(t, os.Chmod(path, 0640))

	again, err := Open(path, testLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })
	require.NoError(t, again.PingContext(context.Background()))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0640), info.Mode().Perm(), "reopening must not touch the mode of an existing file")
}
