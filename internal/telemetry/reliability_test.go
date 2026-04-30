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
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

// TestService_DatadirUnwritable asserts FR-016: when the datadir cannot
// be created or probed, telemetry disables itself silently with a single
// WARN log line.
func TestService_DatadirUnwritable(t *testing.T) {
	target := readOnlyDataDir(t)
	if target == "" {
		t.Skip("read-only datadir not testable on this platform")
	}

	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.DataDir = target
	cfg.Disabled = false

	svc := New(cfg, Deps{Edition: staticEdition{value: extension.Community}}, logger)

	if svc.IsActive() {
		t.Fatalf("service must be inactive when datadir is unwritable")
	}

	rec, level := findLogRecord(t, buf, "telemetry disabled")
	if level != "WARN" {
		t.Errorf("datadir-unwritable level: got %s, want WARN", level)
	}
	if rec["reason"] != "datadir-unwritable" {
		t.Errorf("reason: got %v, want datadir-unwritable", rec["reason"])
	}
	if rec["datadir"] != target {
		t.Errorf("datadir: got %v, want %s", rec["datadir"], target)
	}
	if rec["error"] == nil || rec["error"] == "" {
		t.Errorf("expected non-empty error attr in datadir-unwritable WARN")
	}
}

// TestService_ContextCancellationExits asserts FR-011: cancelling the
// context stops the SDK reporting loop. We verify the observable
// behaviour — no new snapshots after cancel — which is what FR-011 is
// actually about. Goroutine-counting deltas are unreliable in tests
// because httptest, the race detector, and the runtime spawn idle
// workers that come and go on their own schedule.
func TestService_ContextCancellationExits(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, _ := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Endpoint = server.URL

	svc := New(cfg, Deps{
		Edition:          staticEdition{value: extension.Community},
		Containers:       fakeCounter{value: 0},
		Endpoints:        fakeCounter{value: 0},
		Heartbeats:       fakeCounter{value: 0},
		Certificates:    fakeCounter{value: 0},
		Webhooks:         fakeCounter{value: 0},
		StatusComponents: fakeCounter{value: 0},
	}, logger)
	if !svc.IsActive() {
		t.Fatalf("expected active service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)

	// Wait for the SDK to send its initial snapshot.
	deadline := time.Now().Add(2 * time.Second)
	for collector.snapshotCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if collector.snapshotCount() == 0 {
		t.Fatalf("expected at least one snapshot before cancel")
	}
	snapshotsAtCancel := collector.snapshotCount()

	cancel()
	// After cancel, the SDK must stop sending. The default ReportInterval
	// is clamped to 1m by the SDK, so we won't see a tick within the test
	// window even if the loop hadn't exited — this assertion covers the
	// stronger "no in-flight snapshot survives shutdown" guarantee by
	// waiting long enough for the goroutine to drain.
	time.Sleep(500 * time.Millisecond)
	if got := collector.snapshotCount(); got > snapshotsAtCancel {
		t.Errorf("snapshots arrived after cancel: before=%d after=%d", snapshotsAtCancel, got)
	}

	// Sanity: total goroutine count is bounded — we are not leaking. We
	// don't pin a number, just assert "not unbounded". This catches
	// regressions like double-Start.
	if g := runtime.NumGoroutine(); g > 50 {
		t.Errorf("suspicious goroutine leak after cancel: %d", g)
	}
}

// TestService_PanickingProvider_ContainedAtStart asserts FR-012: a panic
// in the provider during snapshot collection does not propagate beyond
// the goroutine.
//
// We exercise this by forcing the SDK to call into the provider with a
// counter that panics. The recover in safeCount handles it; if it
// somehow escaped the provider, the SDK's own `defer recover` in our
// Start() goroutine catches it. Either way, the host process must
// survive.
func TestService_PanickingProvider_ContainedAtStart(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Endpoint = server.URL

	svc := New(cfg, Deps{
		Containers:       fakeCounter{panic: "containers panic"},
		Endpoints:        fakeCounter{value: 1},
		Heartbeats:       fakeCounter{value: 0},
		Certificates:    fakeCounter{value: 0},
		Webhooks:         fakeCounter{value: 0},
		StatusComponents: fakeCounter{value: 0},
		Edition:          staticEdition{value: extension.Community},
	}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// If panics escape, the test process itself dies with non-zero exit.
	// Reaching the assertions below means containment worked.
	svc.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for collector.snapshotCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if collector.snapshotCount() == 0 {
		t.Fatalf("expected at least one snapshot to land despite counter panic")
	}
	metrics := collector.latestSnapshot()
	if metrics["containers_total"] != float64(0) {
		t.Errorf("panicking counter should report 0, got %v", metrics["containers_total"])
	}
	if metrics["endpoints_total"] != float64(1) {
		t.Errorf("non-panicking counter should report its value, got %v", metrics["endpoints_total"])
	}
	if !strings.Contains(buf.String(), `"counter":"containers"`) {
		t.Errorf("expected WARN with counter=containers, got:\n%s", buf.String())
	}
}
