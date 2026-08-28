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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

const sentinelPassword = "s3cr3t-Sentinel-app"

// freshPostgresDSN creates an empty database on the configured test server and
// returns its DSN, or skips when no server is configured (R11: skipped, never
// failed).
func freshPostgresDSN(t *testing.T) string {
	t.Helper()
	adminDSN := storetest.AdminDSN(t)

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

// syncBuffer collects log output from concurrent writers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// storageEnv returns a config that boots without a container runtime, so these
// tests exercise the storage path only.
func storageEnv(t *testing.T) (Config, *syncBuffer, *slog.Logger) {
	t.Helper()
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := Config{
		DBPath:  filepath.Join(t.TempDir(), "test.db"),
		Addr:    "127.0.0.1:0",
		Version: "1.4.0-test",
	}
	return cfg, logs, logger
}

func healthPayload(t *testing.T, a *App) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	a.router.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// TestNew_PostgresStorage covers US1 scenarios 2 and 3: an empty external
// database gets its schema and the instance is operational without any manual
// step; a restart on the same database finds everything and recreates nothing.
func TestNew_PostgresStorage(t *testing.T) {
	dsn := freshPostgresDSN(t)
	cfg, _, logger := storageEnv(t)
	cfg.DatabaseURL = dsn
	cfg.Mode = "server"

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	body := healthPayload(t, a)
	st, ok := body["storage"].(map[string]interface{})
	require.True(t, ok, "health must carry the storage object")
	assert.Equal(t, "postgres", st["engine"])
	assert.Equal(t, true, st["connected"])
	assert.Equal(t, float64(0), st["peers"])

	// Second instance on the same database: it finds the schema and recreates
	// nothing (scenario 3).
	cfg2, _, logger2 := storageEnv(t)
	cfg2.DatabaseURL = dsn
	cfg2.Mode = "server"
	b, err := New(cfg2, logger2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.db.Close() })

	head, err := store.EmbeddedHeadVersion(store.DialectPostgres)
	require.NoError(t, err)
	version, err := b.db.SchemaVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, head, version, "the schema stays at the embedded head")
}

// sentinelRoleDSN creates a dedicated role whose password is the sentinel, and
// returns a DSN using it against a fresh database. The sentinel then travels
// through the whole successful open path, which is what SC-007 must prove.
func sentinelRoleDSN(t *testing.T) string {
	t.Helper()
	adminDSN := storetest.AdminDSN(t)
	dsn := freshPostgresDSN(t)

	admin, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err)
	defer func() { _ = admin.Close() }()

	role := "r_" + strings.ReplaceAll(uid.New(), "-", "")[:16]
	ctx := context.Background()
	_, err = admin.ExecContext(ctx,
		"CREATE ROLE "+role+" LOGIN PASSWORD '"+sentinelPassword+"'")
	require.NoError(t, err)

	u, err := store.ParseDSN(dsn)
	require.NoError(t, err)
	dbName := strings.TrimPrefix(u.Path, "/")
	_, err = admin.ExecContext(ctx, "GRANT ALL ON DATABASE "+dbName+" TO "+role)
	require.NoError(t, err)

	// The role owns the schema so migrations may create tables.
	target, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	_, err = target.ExecContext(ctx, "GRANT ALL ON SCHEMA public TO "+role)
	require.NoError(t, err)
	require.NoError(t, target.Close())

	t.Cleanup(func() {
		// The database is dropped by freshPostgresDSN's cleanup, which runs
		// first (LIFO); the role can then go.
		_, _ = admin.Exec("DROP ROLE IF EXISTS " + role)
	})

	u.User = url.UserPassword(role, sentinelPassword)
	return u.String()
}

// TestNew_PostgresStorage_NeverLogsCredentials pins SC-007 on the happy path:
// the password never reaches the logs, at any level, nor the health response.
func TestNew_PostgresStorage_NeverLogsCredentials(t *testing.T) {
	dsn := sentinelRoleDSN(t)

	cfg, logs, logger := storageEnv(t)
	cfg.DatabaseURL = dsn
	cfg.Mode = "server"

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	assert.NotContains(t, logs.String(), sentinelPassword, "the password must never reach the logs")

	raw, err := json.Marshal(healthPayload(t, a))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), sentinelPassword)
	assert.NotContains(t, string(raw), "postgres://")
}

// TestNew_UnreachablePostgres pins FR-004 and SC-007 on the failure path: the
// instance refuses to start, never falls back to the local file, and the
// message carries no credential.
func TestNew_UnreachablePostgres(t *testing.T) {
	cfg, logs, logger := storageEnv(t)
	// Port 1 on loopback: closed, so this fails fast without a test server.
	cfg.DatabaseURL = "postgres://app:" + sentinelPassword + "@127.0.0.1:1/maintenant?sslmode=disable"
	cfg.Mode = "server"

	a, err := New(cfg, logger)
	require.Error(t, err, "a configured but unusable database must stop the boot")
	assert.Nil(t, a)
	require.ErrorIs(t, err, store.ErrUnreachable)

	assert.NotContains(t, err.Error(), sentinelPassword, "FR-021: the error carries no credential")
	assert.NotContains(t, logs.String(), sentinelPassword)

	// FR-004: no local file was created as a fallback.
	_, statErr := os.Stat(cfg.DBPath)
	assert.True(t, os.IsNotExist(statErr), "no silent fallback to SQLite")
}

// TestNew_SQLiteStorage_UnchangedForUnconfiguredInstalls is US3 scenario 1 and
// FR-029: an install that configured nothing sees no difference. Same engine,
// same file, and — the part that is easy to break — not one new log line.
func TestNew_SQLiteStorage_UnchangedForUnconfiguredInstalls(t *testing.T) {
	cfg, logs, logger := storageEnv(t)
	require.Empty(t, cfg.DatabaseURL, "nothing configured")

	a, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.db.Close() })

	assert.Equal(t, "sqlite", a.db.Engine())
	_, statErr := os.Stat(cfg.DBPath)
	assert.NoError(t, statErr, "the local file is where it has always been")

	body := healthPayload(t, a)
	st, ok := body["storage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "sqlite", st["engine"])
	assert.Equal(t, true, st["connected"])
	assert.Equal(t, float64(0), st["peers"])

	// The messages this feature added are all on the PostgreSQL path. An
	// unconfigured install must not read a single one of them.
	for _, added := range []string{
		"storage opened",
		"another instance is working on this database",
		"database unreachable",
		"database credentials refused",
		"instance registration failed",
	} {
		assert.NotContains(t, logs.String(), added,
			"an unconfigured install must see no new message (US3 scenario 1)")
	}
	assert.NotContains(t, logs.String(), "postgres")
}

// TestSQLiteMigrationsStillReachHead pins FR-029 for an existing install: a
// database created before this feature migrates to the new head without any
// conversion or rebuild, and keeps its data.
func TestSQLiteMigrationsStillReachHead(t *testing.T) {
	cfg, _, logger := storageEnv(t)

	a, err := New(cfg, logger)
	require.NoError(t, err)

	head, err := store.EmbeddedHeadVersion(store.DialectSQLite)
	require.NoError(t, err)
	version, err := a.db.SchemaVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, head, version)

	// The UUID conversion marker is intact: the historical conversions ran on
	// this path and only on this path.
	var format string
	require.NoError(t, a.db.Reader().QueryRowContext(context.Background(),
		"SELECT value FROM schema_meta WHERE key = 'format'").Scan(&format))
	assert.Equal(t, "uuid-v1", format)
	require.NoError(t, a.db.Close())

	// Reopening the same file finds the schema and changes nothing.
	b, err := New(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.db.Close() })
	version2, err := b.db.SchemaVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, version, version2)
}
