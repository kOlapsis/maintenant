// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/mcp"
	"github.com/kolapsis/maintenant/internal/uid"
)

type hold struct {
	held    chan struct{}
	release chan struct{}
	enter   sync.Once
	free    sync.Once
}

func newHold() *hold {
	return &hold{held: make(chan struct{}), release: make(chan struct{})}
}

func (h *hold) wait() {
	h.enter.Do(func() { close(h.held) })
	<-h.release
}

func (h *hold) let() { h.free.Do(func() { close(h.release) }) }

// holdingHandler stops the goroutine that logs msg until the hold is released.
type holdingHandler struct {
	slog.Handler
	msg string
	h   *hold
}

func (l holdingHandler) Handle(ctx context.Context, r slog.Record) error {
	if l.msg != "" && r.Message == l.msg {
		l.h.wait()
	}
	return l.Handler.Handle(ctx, r)
}

func (l holdingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	l.Handler = l.Handler.WithAttrs(attrs)
	return l
}

func (l holdingHandler) WithGroup(name string) slog.Handler {
	l.Handler = l.Handler.WithGroup(name)
	return l
}

func asPro(t *testing.T) {
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

// heldSMTP accepts one connection and answers nothing until the hold is released.
func heldSMTP(t *testing.T, h *hold) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		h.wait()
		_ = conn.Close()
	}()
	return ln.Addr().String()
}

func TestShutdown_WaitsForEveryServiceGoroutine(t *testing.T) {
	statusPage := extpoint.Set{StatusPage: commercial.Extensions().StatusPage}
	cases := []struct {
		name  string
		hold  string
		setup func(t *testing.T, cfg *Config, h *hold) []Option
		begin func(t *testing.T, a *App, logs *syncBuffer)
	}{
		{
			name: "alert engine event loop",
			hold: "alert: dedup, skipping duplicate",
			begin: func(_ *testing.T, a *App, _ *syncBuffer) {
				evt := alert.Event{Source: alert.SourceEndpoint, AlertType: "shutdown_probe", Severity: alert.SeverityWarning,
					EntityType: "endpoint", EntityID: "probe", EntityName: "probe", Message: "probe", Timestamp: time.Now()}
				a.alertEngine.EventChannel() <- evt
				a.alertEngine.EventChannel() <- evt
			},
		},
		{
			name: "alert engine escalation evaluator",
			hold: "alert engine: escalation evaluator started",
			setup: func(t *testing.T, _ *Config, _ *hold) []Option {
				asPro(t)
				return []Option{WithExtensions(extpoint.Set{Escalation: commercial.Extensions().Escalation})}
			},
		},
		{name: "heartbeat deadline checker", hold: "heartbeat: deadline checker started"},
		{name: "outbound heartbeat sender", hold: "outbound heartbeat: sender started"},
		{name: "resource rollup loop", hold: "starting resource rollup loop"},
		{
			name: "triggered update scan",
			hold: "starting update scan",
			begin: func(t *testing.T, a *App, logs *syncBuffer) {
				require.Eventually(t, func() bool { return strings.Contains(logs.String(), "starting update intelligence service") },
					10*time.Second, 10*time.Millisecond)
				_, err := a.updateSvc.TriggerScan(context.Background())
				require.NoError(t, err)
			},
		},
		{
			name: "status page subscriber notification",
			hold: "status: sending notifications",
			setup: func(t *testing.T, cfg *Config, _ *hold) []Option {
				asPro(t)
				cfg.SMTP = SMTPConfig{Host: "127.0.0.1", Port: "1", From: "status@example.com"}
				return []Option{WithExtensions(statusPage)}
			},
			begin: func(_ *testing.T, a *App, _ *syncBuffer) {
				a.statusSvc.NotifySubscribers(context.Background(), "subject", "message")
			},
		},
		{
			name: "status page confirmation email",
			setup: func(t *testing.T, cfg *Config, h *hold) []Option {
				asPro(t)
				host, port, err := net.SplitHostPort(heldSMTP(t, h))
				require.NoError(t, err)
				cfg.SMTP = SMTPConfig{Host: host, Port: port, From: "status@example.com", AllowPlaintext: true}
				return []Option{WithExtensions(statusPage)}
			},
			begin: func(t *testing.T, a *App, _ *syncBuffer) {
				require.NoError(t, a.subscriberSvc.Subscribe(context.Background(), "someone@example.com"))
			},
		},
		{
			name: "MCP socket session",
			hold: "heartbeat: paused",
			setup: func(_ *testing.T, cfg *Config, _ *hold) []Option {
				cfg.MCP.Socket = true
				return nil
			},
			begin: func(t *testing.T, a *App, _ *syncBuffer) {
				hb, err := a.heartbeatSvc.CreateHeartbeat(context.Background(),
					heartbeat.CreateHeartbeatInput{Name: "probe", IntervalSeconds: 3600, GraceSeconds: 600}, uid.New())
				require.NoError(t, err)
				conn, err := net.Dial("unix", mcp.SocketPath(a.cfg.DBPath))
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				client := gomcp.NewClient(&gomcp.Implementation{Name: "probe", Version: "0"}, nil)
				cs, err := client.Connect(context.Background(), &gomcp.IOTransport{Reader: conn, Writer: conn}, nil)
				require.NoError(t, err)
				go func() {
					_, _ = cs.CallTool(context.Background(), &gomcp.CallToolParams{Name: "pause_monitor",
						Arguments: map[string]any{"monitor_type": "heartbeat", "monitor_id": hb.ID}})
				}()
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, _ := storageEnv(t)
			h := newHold()
			t.Cleanup(h.let)
			var opts []Option
			if tc.setup != nil {
				opts = tc.setup(t, &cfg, h)
			}
			logs := &syncBuffer{}
			logger := slog.New(holdingHandler{
				Handler: slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
				msg:     tc.hold,
				h:       h,
			})
			a, err := New(cfg, logger, opts...)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- a.Start(ctx) }()
			require.Eventually(t, func() bool { return strings.Contains(logs.String(), "starting HTTP server") },
				10*time.Second, 10*time.Millisecond)
			if tc.begin != nil {
				tc.begin(t, a, logs)
			}
			select {
			case <-h.held:
			case <-time.After(10 * time.Second):
				t.Fatal("the goroutine never reached its hold")
			}

			cancel()
			select {
			case <-done:
				t.Fatal("Start returned while the goroutine was still running")
			case <-time.After(300 * time.Millisecond):
			}
			h.let()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(20 * time.Second):
				t.Fatal("Start did not return once the goroutine was released")
			}
		})
	}
}

func TestShutdown_StopsTheMCPOAuthCleanup(t *testing.T) {
	cfg, logs, logger := storageEnv(t)
	cfg.MCP.Enabled = true
	cfg.MCP.ClientID = "maintenant-test"
	cfg.MCP.ClientSecret = "a-long-and-random-enough-client-secret-0123456789"
	cfg.BaseURL = "http://127.0.0.1"
	a, err := New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	require.Eventually(t, func() bool { return strings.Contains(logs.String(), "starting HTTP server") },
		10*time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	buf := make([]byte, 1<<22)
	stacks := string(buf[:runtime.Stack(buf, true)])
	assert.False(t, strings.Contains(stacks, "internal/mcp/oauth.StartCleanup"), "the OAuth cleanup outlived the shutdown")
}
