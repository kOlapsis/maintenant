package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnsetSettingsLeaveConfigAtItsFormerDefaults(t *testing.T) {
	for _, key := range []string{
		"MAINTENANT_STATE_DIR",
		"MAINTENANT_SQLITE_SYNCHRONOUS",
		"MAINTENANT_REQUIRE_STATE_DIR",
		"MAINTENANT_REQUIRE_EXISTING_DATA",
		"MAINTENANT_DB",
		"MAINTENANT_DATABASE_URL",
		"MAINTENANT_TELEMETRY_DATADIR",
	} {
		t.Setenv(key, "")
	}

	cfg := ConfigFromEnv()
	assert.Empty(t, cfg.StateDir)
	assert.Empty(t, cfg.SQLiteSynchronous)
	assert.False(t, cfg.RequireStateDir)
	assert.False(t, cfg.RequireExistingData)
	assert.Equal(t, "./maintenant.db", cfg.DBPath)
	require.NoError(t, cfg.ValidateStorage())
}

func TestUnsetSettingsLeavePathsWhereTheyWere(t *testing.T) {
	t.Setenv("MAINTENANT_TELEMETRY_DATADIR", "")
	t.Setenv("SQLITE_TMPDIR", "/former/tmp")

	root, err := ResolveStateRoot(Config{DBPath: "./maintenant.db"})
	require.NoError(t, err)
	require.NoError(t, prepareStateRoot(root))

	assert.Equal(t, "./maintenant.db", root.DBPath)
	assert.Equal(t, ".", root.LicenseDir, "the licence cache sat next to the database file")
	assert.Equal(t, filepath.Join(".", "embedded-agent"), root.EmbeddedAgentDir)
	assert.Equal(t, "/data/shm", root.TelemetryDir)
	assert.Equal(t, "/former/tmp", os.Getenv("SQLITE_TMPDIR"), "SQLITE_TMPDIR is not ours to set")
}

func TestUnsetSettingsLeaveTheSQLiteDSNUnchanged(t *testing.T) {
	cfg, _, logger := storageEnv(t)

	db, err := openStorage(t.Context(), cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var synchronous int
	require.NoError(t, db.ReadDB().QueryRow("PRAGMA synchronous").Scan(&synchronous))
	assert.Equal(t, 1, synchronous)

	var journal string
	require.NoError(t, db.ReadDB().QueryRow("PRAGMA journal_mode").Scan(&journal))
	assert.Equal(t, "wal", strings.ToLower(journal))

	level, err := store.NormalizeSynchronous(cfg.SQLiteSynchronous)
	require.NoError(t, err)
	assert.Equal(t, store.SynchronousNormal, level)
}

func TestUnsetSettingsStartTheServerAlone(t *testing.T) {
	cfg, _, logger := storageEnv(t)

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	assert.Empty(t, a.stateRoot.Dir)
	assert.Equal(t, cfg.DBPath, a.cfg.DBPath)
	version, err := a.db.SchemaVersion(t.Context())
	require.NoError(t, err)
	assert.NotZero(t, version, "a first install still creates its own schema")
}

func TestProductCodeNeverMentionsTheClusterManager(t *testing.T) {
	var offenders []string
	for _, dir := range []string{"internal", "cmd"} {
		require.NoError(t, filepath.Walk(filepath.Join("..", "..", dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			content, readErr := os.ReadFile(path) // #nosec G304 -- test walking the repository
			if readErr != nil {
				return readErr
			}
			if strings.Contains(strings.ToLower(string(content)), "opensvc") {
				offenders = append(offenders, path)
			}
			return nil
		}))
	}
	assert.Empty(t, offenders, "the binary must not know it may be run under a cluster manager: %v", offenders)
}
