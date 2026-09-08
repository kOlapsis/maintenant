package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRootWithoutStateDirKeepsHistoricalPaths(t *testing.T) {
	t.Setenv("MAINTENANT_TELEMETRY_DATADIR", "")

	root, err := ResolveStateRoot(Config{DBPath: "/var/lib/maintenant/maintenant.db"})
	require.NoError(t, err)

	assert.Empty(t, root.Dir)
	assert.Equal(t, "/var/lib/maintenant/maintenant.db", root.DBPath)
	assert.Equal(t, "/var/lib/maintenant", root.LicenseDir)
	assert.Equal(t, "/var/lib/maintenant/embedded-agent", root.EmbeddedAgentDir)
	assert.Equal(t, "/data/shm", root.TelemetryDir)
	assert.Empty(t, root.TLSDir, "no TLS directory is implied without a state root")
	assert.Empty(t, root.TmpDir, "SQLITE_TMPDIR is left alone without a state root")
}

func TestStateRootWithoutStateDirHonoursTelemetryDataDir(t *testing.T) {
	t.Setenv("MAINTENANT_TELEMETRY_DATADIR", "/srv/shm")

	root, err := ResolveStateRoot(Config{DBPath: "./maintenant.db"})
	require.NoError(t, err)
	assert.Equal(t, "/srv/shm", root.TelemetryDir)
	assert.Equal(t, ".", root.LicenseDir)
}

func TestStateRootPlacesEveryPathUnderTheRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MAINTENANT_TELEMETRY_DATADIR", "/srv/shm")

	root, err := ResolveStateRoot(Config{StateDir: dir, DBPath: "./maintenant.db"})
	require.NoError(t, err)

	assert.Equal(t, dir, root.Dir)
	assert.Equal(t, filepath.Join(dir, "maintenant.db"), root.DBPath)
	assert.Equal(t, dir, root.LicenseDir)
	assert.Equal(t, filepath.Join(dir, "embedded-agent"), root.EmbeddedAgentDir)
	assert.Equal(t, filepath.Join(dir, "shm"), root.TelemetryDir,
		"the state root overrides MAINTENANT_TELEMETRY_DATADIR")
	assert.Equal(t, filepath.Join(dir, "tls"), root.TLSDir)
	assert.Equal(t, filepath.Join(dir, "tmp"), root.TmpDir)
}

func TestStateRootYieldsToAnExplicitAbsoluteDatabasePath(t *testing.T) {
	dir := t.TempDir()

	root, err := ResolveStateRoot(Config{StateDir: dir, DBPath: "/srv/data/other.db"})
	require.NoError(t, err)

	assert.Equal(t, "/srv/data/other.db", root.DBPath)
	assert.Equal(t, dir, root.LicenseDir)
	assert.Equal(t, filepath.Join(dir, "embedded-agent"), root.EmbeddedAgentDir)
}

func TestStateRootWithPostgresIgnoresTheUnusedDatabasePath(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		StateDir:    dir,
		DBPath:      "./maintenant.db",
		DatabaseURL: "postgres://maintenant@db.internal:5432/maintenant",
	}
	root, err := ResolveStateRoot(cfg)
	require.NoError(t, err)

	for _, path := range []string{root.LicenseDir, root.EmbeddedAgentDir, root.TelemetryDir, root.TmpDir} {
		assert.True(t, filepath.IsAbs(path), "%q must be absolute, not relative to the working directory", path)
		assert.Equal(t, dir, commonRoot(dir, path), "%q must sit under the state root", path)
	}
}

func TestStateRootRejectsAMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")

	_, err := ResolveStateRoot(Config{StateDir: missing, DBPath: "./maintenant.db"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
	assert.Contains(t, err.Error(), "MAINTENANT_STATE_DIR")
}

func TestStateRootRejectsAnUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a 0500 directory anyway")
	}
	dir := filepath.Join(t.TempDir(), "readonly")
	require.NoError(t, os.Mkdir(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := ResolveStateRoot(Config{StateDir: dir, DBPath: "./maintenant.db"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not writable")
}

func TestStateRootRejectsARelativeDirectory(t *testing.T) {
	_, err := ResolveStateRoot(Config{StateDir: "state", DBPath: "./maintenant.db"})
	require.ErrorIs(t, err, ErrStateDirNotAbsolute)
}

func TestStateRootRejectsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := ResolveStateRoot(Config{StateDir: file, DBPath: "./maintenant.db"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestPrepareStateRootPointsSQLiteTemporariesAtTheRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SQLITE_TMPDIR", "")

	root, err := ResolveStateRoot(Config{StateDir: dir, DBPath: "./maintenant.db"})
	require.NoError(t, err)
	require.NoError(t, prepareStateRoot(root))

	assert.Equal(t, root.TmpDir, os.Getenv("SQLITE_TMPDIR"))
	info, err := os.Stat(root.TmpDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestPrepareStateRootLeavesSQLiteTemporariesAloneWithoutARoot(t *testing.T) {
	t.Setenv("SQLITE_TMPDIR", "/somewhere/else")

	root, err := ResolveStateRoot(Config{DBPath: "./maintenant.db"})
	require.NoError(t, err)
	require.NoError(t, prepareStateRoot(root))

	assert.Equal(t, "/somewhere/else", os.Getenv("SQLITE_TMPDIR"))
}

func commonRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) ||
		(len(rel) > 2 && rel[:3] == "../") {
		return path
	}
	return root
}
