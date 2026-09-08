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

var knownLocalState = map[string]string{
	".maintenant-license":          "signed licence cache: re-verified online at startup, Community while offline without it",
	".maintenant-update-window":    "update window record: a fresh grace window opens, which plays in the operator's favour",
	"shm/shm_identity.json":        "telemetry identity: losing it only breaks the continuity of anonymous counters",
	"embedded-agent/identity.json": "embedded agent key pair: a new one means a new agent, and the old one lingers in the fleet",
}

func TestServerKeepsNoUndocumentedLocalState(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("MAINTENANT_DATA_DIR", stateDir)
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	cfg, _, logger := storageEnv(t)
	cfg.StateDir = stateDir
	cfg.DBPath = "./maintenant.db"

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	dbName := filepath.Base(a.cfg.DBPath)
	var found []string
	require.NoError(t, filepath.WalkDir(stateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch d.Name() {
		case dbName, dbName + "-wal", dbName + "-shm":
			return nil
		}
		rel, relErr := filepath.Rel(stateDir, path)
		if relErr != nil {
			return relErr
		}
		if _, known := knownLocalState[filepath.ToSlash(rel)]; !known {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	}))

	sort.Strings(found)
	assert.Empty(t, found,
		"the server wrote local state that is neither in the database nor documented as having to follow the instance (FR-010): %v", found)
}

func TestKnownLocalStateIsDocumented(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", "postgresql.md"))
	require.NoError(t, err, "the guide must exist: it is where FR-010's consequence is written")

	for file := range knownLocalState {
		assert.Contains(t, string(guide), file,
			"%s is kept outside the database and must be named in docs/guides/postgresql.md", file)
	}
}
