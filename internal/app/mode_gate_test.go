package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/extension"
)

// gateErrFragment is the part of the mode-gate error these tests key on.
const gateErrFragment = "requires the Pro edition"

func withEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func modeGateCfg(t *testing.T, mode string) (app.Config, *slog.Logger) {
	t.Helper()
	t.Setenv("MAINTENANT_RUNTIME", "docker")
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-test-socket-abc123.sock")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", "")

	cfg := app.Config{
		DBPath: filepath.Join(t.TempDir(), "test.db"),
		Addr:   freePort(t),
		Mode:   mode,
	}
	cfg.MultiHost.GRPCListen = freePort(t)
	cfg.MultiHost.InsecureGRPC = true // skip TLS material generation in tests
	return cfg, slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startAndCollect runs Start with a bounded context and returns its error.
func startAndCollect(t *testing.T, a *app.App, grace time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	select {
	case err := <-done:
		return err // Start returned on its own (the gate rejected it)
	case <-time.After(grace):
		cancel() // it got past the gate and is running — shut it down
	}

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
		return nil
	}
}

// TestNew_ServerMode_DoesNotGateOnEdition is the regression guard.
//
// The gate used to sit in New(), *before* extension.CurrentEdition was wired to
// the license manager. At that point it read the package default — community —
// so `--mode=server` called os.Exit(1) for every edition, Pro included: a paying
// customer following the multi-host guide could never boot.
//
// The edition is deliberately NOT overridden here. Overriding it to Pro is what
// hid the bug in the first place: it makes the old in-New() check pass and tests
// nothing. Left at the package default, this test kills the test binary against
// the old code (os.Exit) and passes against the fix.
func TestNew_ServerMode_DoesNotGateOnEdition(t *testing.T) {
	require.Equal(t, extension.Community, extension.CurrentEdition(),
		"precondition: this test must run at the package default edition")

	cfg, logger := modeGateCfg(t, "server")

	a, err := app.New(cfg, logger)
	require.NoError(t, err, "New() must not gate on edition — the license is not loaded yet")
	require.NotNil(t, a)
}

// TestStart_ServerMode_RejectedOutsidePro: the gate still guards, and reports
// rather than calling os.Exit — the caller decides what to do with the error.
func TestStart_ServerMode_RejectedOutsidePro(t *testing.T) {
	withEdition(t, extension.Community)
	cfg, logger := modeGateCfg(t, "server")

	a, err := app.New(cfg, logger)
	require.NoError(t, err)

	err = startAndCollect(t, a, 5*time.Second)
	require.Error(t, err, "server mode must be refused outside Pro")
	assert.Contains(t, err.Error(), gateErrFragment)
	assert.Contains(t, err.Error(), string(extension.Community),
		"the refusal must name the edition actually in force")
}

// TestStart_ServerMode_AllowedInPro is the case that never worked: a valid Pro
// license must let server mode boot.
func TestStart_ServerMode_AllowedInPro(t *testing.T) {
	withEdition(t, extension.Pro)
	cfg, logger := modeGateCfg(t, "server")

	a, err := app.New(cfg, logger)
	require.NoError(t, err)

	err = startAndCollect(t, a, 2*time.Second)
	if err != nil {
		assert.NotContains(t, err.Error(), gateErrFragment,
			"server mode must not be refused under a Pro edition")
	}
}

// TestStart_EmbeddedMode_NotGated: the default mode is never subject to the gate.
func TestStart_EmbeddedMode_NotGated(t *testing.T) {
	for _, mode := range []string{"", "embedded"} {
		t.Run("mode="+mode, func(t *testing.T) {
			withEdition(t, extension.Community)
			cfg, logger := modeGateCfg(t, mode)

			a, err := app.New(cfg, logger)
			require.NoError(t, err)

			err = startAndCollect(t, a, 2*time.Second)
			if err != nil {
				assert.False(t, strings.Contains(err.Error(), gateErrFragment),
					"embedded mode must never hit the mode gate")
			}
		})
	}
}
