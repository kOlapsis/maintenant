package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireStateDirRefusesToStartWithoutARoot(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	cfg.RequireStateDir = true

	_, err := New(cfg, logger)
	require.ErrorIs(t, err, ErrStateDirRequired)
	assert.Contains(t, err.Error(), "MAINTENANT_STATE_DIR")
}

func TestRequireStateDirIsSatisfiedByARoot(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	cfg.RequireStateDir = true
	cfg.StateDir = t.TempDir()
	cfg.DBPath = "./maintenant.db" // the default: the root takes it over

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })
	assert.Equal(t, filepath.Join(cfg.StateDir, "maintenant.db"), a.cfg.DBPath)
}

func TestRequireExistingDataRefusesAFreshDataSet(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	cfg.RequireExistingData = true

	_, err := New(cfg, logger)
	require.ErrorIs(t, err, ErrDataSetIsNew)
	assert.Contains(t, err.Error(), cfg.DBPath, "the message must name the data set it refused")
	assert.Contains(t, err.Error(), "MAINTENANT_REQUIRE_EXISTING_DATA")
}

func TestRequireExistingDataAcceptsAMigratedDataSet(t *testing.T) {
	cfg, _, logger := storageEnv(t)

	first, err := New(cfg, logger)
	require.NoError(t, err)
	require.NoError(t, first.db.Close())

	cfg.RequireExistingData = true
	second, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.db.Close() })
}

func TestStartupWithoutGuardsCreatesTheDataSet(t *testing.T) {
	cfg, _, logger := storageEnv(t)

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	version, err := a.db.SchemaVersion(t.Context())
	require.NoError(t, err)
	assert.NotZero(t, version)
}
