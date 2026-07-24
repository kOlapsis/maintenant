package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
)

func degradedEnv(t *testing.T, tmpDir string) (app.Config, *slog.Logger) {
	t.Helper()
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")
	return app.Config{
		DBPath: filepath.Join(tmpDir, "test.db"),
		Addr:   "127.0.0.1:0",
	}, slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestNew_DegradedBoot (T005/T006): app.New() must succeed even when runtime is unreachable.
func TestNew_DegradedBoot(t *testing.T) {
	cfg, logger := degradedEnv(t, t.TempDir())
	a, err := app.New(cfg, logger)
	require.NoError(t, err, "app.New() must not return an error when runtime is unreachable")
	assert.NotNil(t, a, "app.New() must return a non-nil *App")
}

// TestStart_DegradedMode (T008): Start() must launch non-container services and expose
// the HTTP server even when the container runtime is unreachable.
func TestStart_DegradedMode(t *testing.T) {
	// Find a free port before starting.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	_ = ln.Close()

	tmpDir := t.TempDir()
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := app.Config{
		DBPath: filepath.Join(tmpDir, "test.db"),
		Addr:   addr,
	}

	a, err := app.New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	// Wait until the HTTP server is reachable.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/api/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, "health endpoint must return 200 in degraded mode")
			cancel() // trigger graceful shutdown
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Start() should return (shutdown) within the context.
	select {
	case err := <-done:
		assert.NoError(t, err, "Start() must exit cleanly on context cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}
