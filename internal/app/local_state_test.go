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

package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownLocalState is every file the server may keep outside the database, and
// the reason it is allowed to. FR-010 forbids keeping anything a replacement
// instance would need; each entry here is documented in
// docs/guides/postgresql.md as having to follow the instance.
//
// This list IS the documentation contract. A new file appearing here fails the
// test with its name, so the decision — carry it in the database, or document
// it as following the instance — is made deliberately, not by accident.
var knownLocalState = map[string]string{
	".maintenant-license":       "signed licence cache: re-verified online at startup, Community while offline without it",
	".maintenant-update-window": "update window record: a fresh grace window opens, which plays in the operator's favour",
}

// TestServerKeepsNoUndocumentedLocalState pins FR-010: whatever the server
// writes next to itself is either in the database or on the documented list.
func TestServerKeepsNoUndocumentedLocalState(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MAINTENANT_DATA_DIR", dataDir)
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	cfg, _, logger := storageEnv(t)
	cfg.DBPath = filepath.Join(dataDir, "maintenant.db")

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	var found []string
	require.NoError(t, filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		// The SQLite database and its WAL companions are the storage itself,
		// which is exactly what an external database replaces.
		switch {
		case name == filepath.Base(cfg.DBPath),
			name == filepath.Base(cfg.DBPath)+"-wal",
			name == filepath.Base(cfg.DBPath)+"-shm":
			return nil
		}
		if _, known := knownLocalState[name]; !known {
			found = append(found, name)
		}
		return nil
	}))

	sort.Strings(found)
	assert.Empty(t, found,
		"the server wrote local state that is neither in the database nor documented as having to follow the instance (FR-010): %v", found)
}

// TestKnownLocalStateIsDocumented keeps the list above and the guide honest:
// every entry carries the consequence an operator reads when the data
// directory does not follow the instance.
func TestKnownLocalStateIsDocumented(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", "postgresql.md"))
	require.NoError(t, err, "the guide must exist: it is where FR-010's consequence is written")

	for file := range knownLocalState {
		assert.Contains(t, string(guide), file,
			"%s is kept outside the database and must be named in docs/guides/postgresql.md", file)
	}
}
