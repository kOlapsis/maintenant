// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/commercial"
	"github.com/kolapsis/maintenant/internal/extension"
)

// Acceptance tests of 045-mcp-hardening on the socket wiring: the socket exists
// only when MCP is wanted, and is closed to other users.

// startUntilHTTPIsUp runs Start and waits for the health route, which the app serves after the socket decision.
func startUntilHTTPIsUp(t *testing.T, a *app.App, addr string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Error("Start() did not return after context cancellation")
		}
	})
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/api/v1/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 50*time.Millisecond, "the HTTP server must become reachable")
}

// US4 scenario 1, FR-030, SC-004, edge case "mode démo": no MCP variable, no socket on disk.
func TestStart_NoMCPVariableMeansNoSocket(t *testing.T) {
	for name, edit := range map[string]func(*app.Config){
		"server":        func(*app.Config) {},
		"demo image":    func(c *app.Config) { c.DemoMode = true },
		"read-only set": func(c *app.Config) { c.MCP.ReadOnly = true },
	} {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg, logger := degradedEnv(t, tmpDir)
			cfg.Addr = freePort(t)
			edit(&cfg)
			a, err := app.New(cfg, logger)
			require.NoError(t, err)

			startUntilHTTPIsUp(t, a, cfg.Addr)
			_, err = os.Stat(filepath.Join(tmpDir, "mcp.sock"))
			assert.True(t, os.IsNotExist(err), "no socket may exist when MCP is not wanted: %v", err)
		})
	}
}

// US4 scenario 2, FR-030: either variable serves the socket next to the database, mode 0600, and it answers.
func TestStart_MCPVariableServesAClosedSocket(t *testing.T) {
	for name, edit := range map[string]func(*app.Config){
		"MAINTENANT_MCP_SOCKET":    func(c *app.Config) { c.MCP.Socket = true },
		"MAINTENANT_MCP":           func(c *app.Config) { c.MCP.Enabled = true; c.MCP.AllowUnauthenticated = true },
		"socket on the demo image": func(c *app.Config) { c.MCP.Socket = true; c.DemoMode = true },
	} {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg, logger := degradedEnv(t, tmpDir)
			cfg.Addr = freePort(t)
			edit(&cfg)
			a, err := app.New(cfg, logger)
			require.NoError(t, err)

			startUntilHTTPIsUp(t, a, cfg.Addr)
			path := filepath.Join(tmpDir, "mcp.sock")
			info, err := os.Stat(path)
			require.NoError(t, err, "the socket sits next to the database")
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			conn, err := net.Dial("unix", path)
			require.NoError(t, err, "the socket answers")
			_ = conn.Close()
		})
	}
}

// Edge case "socket déjà présent au démarrage": a stale file is replaced.
func TestStart_ReplacesAStaleSocket(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, logger := degradedEnv(t, tmpDir)
	cfg.Addr = freePort(t)
	cfg.MCP.Socket = true
	path := filepath.Join(tmpDir, "mcp.sock")
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	a, err := app.New(cfg, logger)
	require.NoError(t, err)
	startUntilHTTPIsUp(t, a, cfg.Addr)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSocket, info.Mode().Type(), "the stale regular file became the socket")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// US4 scenario 3, FR-030: agent mode never serves a socket, variables or not.
func TestStart_AgentModeNeverServesASocket(t *testing.T) {
	withEdition(t, extension.Pro)
	cfg, logger := modeGateCfg(t, "agent")
	cfg.MCP.Socket = true
	cfg.MCP.Enabled = true
	cfg.MCP.AllowUnauthenticated = true
	a, err := app.New(cfg, logger, app.WithExtensions(commercial.Extensions()))
	require.NoError(t, err)

	_ = startAndCollect(t, a, 2*time.Second)
	_, err = os.Stat(filepath.Join(filepath.Dir(cfg.DBPath), "mcp.sock"))
	assert.True(t, os.IsNotExist(err), "agent mode has no socket: %v", err)
}
