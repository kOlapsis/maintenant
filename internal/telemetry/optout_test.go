// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"context"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

// TestService_OptOut_NoSDKInit asserts that with cfg.Disabled = true the
// returned Service holds no SDK client and Start is a no-op.
func TestService_OptOut_NoSDKInit(t *testing.T) {
	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Disabled = true
	deps := Deps{Edition: staticEdition{value: extension.Community}}

	beforeGoroutines := runtime.NumGoroutine()
	svc := New(cfg, deps, logger)
	if svc.IsActive() {
		t.Fatalf("opted-out service should not be active, logs:\n%s", buf.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.Start(ctx)

	// Allow scheduler to settle. Tolerance of 2 covers runtime jitter.
	time.Sleep(100 * time.Millisecond)
	afterGoroutines := runtime.NumGoroutine()
	if afterGoroutines > beforeGoroutines+2 {
		t.Errorf("opt-out should not spawn goroutines: before=%d after=%d", beforeGoroutines, afterGoroutines)
	}
}

// TestService_OptOut_NoOutbound asserts that no request hits the fake
// collector when the operator opts out (validates SC-003 directly).
func TestService_OptOut_NoOutbound(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, _ := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Disabled = true
	cfg.Endpoint = server.URL

	svc := New(cfg, Deps{Edition: staticEdition{value: extension.Community}}, logger)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.Start(ctx)

	// Wait long enough for the SDK to have made a request if it were
	// spawned — but it must not be spawned at all.
	time.Sleep(500 * time.Millisecond)

	if got := collector.snapshotCount(); got != 0 {
		t.Errorf("opt-out should produce zero snapshots, got %d", got)
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.register != 0 || collector.activate != 0 {
		t.Errorf("opt-out should produce zero register/activate calls, got register=%d activate=%d",
			collector.register, collector.activate)
	}
}

// TestService_OptOut_LogShape asserts the exact attributes of the single
// startup log line per FR-017.
func TestService_OptOut_LogShape(t *testing.T) {
	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Disabled = true

	_ = New(cfg, Deps{Edition: staticEdition{value: extension.Community}}, logger)

	rec, level := findLogRecord(t, buf, "telemetry disabled")
	if level != "INFO" {
		t.Errorf("opt-out level: got %s, want INFO", level)
	}
	if rec["reason"] != "opt-out" {
		t.Errorf("opt-out reason: got %v, want opt-out", rec["reason"])
	}
	if _, has := rec["endpoint"]; has {
		t.Errorf("opt-out log must not include endpoint attr: %v", rec["endpoint"])
	}
	if _, has := rec["datadir"]; has {
		t.Errorf("opt-out log must not include datadir attr: %v", rec["datadir"])
	}
}

// TestService_DefaultBoot_NotDisabled validates FR-007: with the env var
// unset, telemetry initialises normally.
func TestService_DefaultBoot_NotDisabled(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, _ := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Disabled = false
	cfg.Endpoint = server.URL

	svc := New(cfg, Deps{
		Containers:       fakeCounter{value: 0},
		Endpoints:        fakeCounter{value: 0},
		Heartbeats:       fakeCounter{value: 0},
		Certificates:    fakeCounter{value: 0},
		Webhooks:         fakeCounter{value: 0},
		StatusComponents: fakeCounter{value: 0},
		Edition:          staticEdition{value: extension.Community},
	}, logger)

	if !svc.IsActive() {
		t.Fatalf("default boot should be active")
	}
}
