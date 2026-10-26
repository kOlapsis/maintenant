// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: an agent starts on a data directory that does not exist yet.

// FR-009, US4 scenario 1: a missing MAINTENANT_DATA_DIR is created, 0700, and the identity lands in it.
func TestLoadOrCreate_CreatesAMissingDataDir(t *testing.T) {
	for _, rel := range []string{"neuf", filepath.Join("deeper", "agent-data")} {
		t.Run(rel, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), rel)

			id, err := LoadOrCreate(dir)
			require.NoError(t, err)
			assert.NotEmpty(t, id.AgentID)

			info, err := os.Stat(dir)
			require.NoError(t, err)
			require.True(t, info.IsDir())
			assert.Equal(t, os.FileMode(0700), info.Mode().Perm())

			file, err := os.Stat(filepath.Join(dir, identityFile))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), file.Mode().Perm())

			again, err := LoadOrCreate(dir)
			require.NoError(t, err)
			assert.Equal(t, id.AgentID, again.AgentID, "the identity written there is the one reloaded")
		})
	}
}

// FR-009, US4 scenario 1: a directory that cannot be created yields an error naming the path.
func TestLoadOrCreate_NamesTheDirectoryItCannotCreate(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0600))
	dir := filepath.Join(blocker, "data")

	_, err := LoadOrCreate(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), dir)
}
