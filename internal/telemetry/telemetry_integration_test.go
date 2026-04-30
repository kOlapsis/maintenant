// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

// fakeCollector stands in for metrics.kolapsis.com. It accepts /v1/register,
// /v1/activate, and /v1/snapshot and records what it received.
type fakeCollector struct {
	mu        sync.Mutex
	register  int
	activate  int
	snapshots []map[string]any
}

func (f *fakeCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/register":
		f.register++
		w.WriteHeader(http.StatusCreated)
	case "/v1/activate":
		f.activate++
		w.WriteHeader(http.StatusOK)
	case "/v1/snapshot":
		body, _ := io.ReadAll(r.Body)
		var envelope struct {
			InstanceID string          `json:"instance_id"`
			Metrics    json.RawMessage `json:"metrics"`
		}
		_ = json.Unmarshal(body, &envelope)
		var metrics map[string]any
		// SDK serialises metrics as raw bytes of the map JSON.
		_ = json.Unmarshal(envelope.Metrics, &metrics)
		f.snapshots = append(f.snapshots, metrics)
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeCollector) snapshotCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.snapshots)
}

func (f *fakeCollector) latestSnapshot() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) == 0 {
		return nil
	}
	return f.snapshots[len(f.snapshots)-1]
}

func TestTelemetryIntegration_DefaultBoot_SendsRecord(t *testing.T) {
	collector := &fakeCollector{}
	server := httptest.NewServer(collector)
	t.Cleanup(server.Close)

	logger, buf := setupTestLogger()
	cfg := newTestConfig(t)
	cfg.Endpoint = server.URL
	cfg.AppVersion = "v0.0.0-test"

	deps := Deps{
		Containers:       fakeCounter{value: 3},
		Endpoints:        fakeCounter{value: 4},
		Heartbeats:       fakeCounter{value: 2},
		Certificates:    fakeCounter{value: 1},
		Webhooks:         fakeCounter{value: 1},
		StatusComponents: fakeCounter{value: 5},
		Edition:          staticEdition{value: extension.Community},
	}

	svc := New(cfg, deps, logger)
	if !svc.IsActive() {
		t.Fatalf("expected active service with writable temp datadir, logs:\n%s", buf.String())
	}

	rec, level := findLogRecord(t, buf, "telemetry enabled")
	if level != "INFO" {
		t.Errorf("expected INFO level, got %s", level)
	}
	if rec["endpoint"] != server.URL {
		t.Errorf("endpoint attr: got %v, want %s", rec["endpoint"], server.URL)
	}
	if rec["edition"] != "community" {
		t.Errorf("edition attr: got %v, want community", rec["edition"])
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.Start(ctx)

	// SDK fires one snapshot immediately on Start before entering its ticker
	// loop. Wait briefly for it to land.
	deadline := time.Now().Add(3 * time.Second)
	for collector.snapshotCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if got := collector.snapshotCount(); got == 0 {
		t.Fatalf("no snapshot received after 3s")
	}
	metrics := collector.latestSnapshot()
	for _, key := range expectedKeys {
		if _, ok := metrics[key]; !ok {
			t.Errorf("snapshot missing key %q", key)
		}
	}
	if metrics["edition"] != "community" {
		t.Errorf("snapshot edition: got %v, want community", metrics["edition"])
	}
	if v, ok := metrics["containers_total"].(float64); !ok || int(v) != 3 {
		t.Errorf("snapshot containers_total: got %v, want 3", metrics["containers_total"])
	}
	// SDK runtime context is included since CollectSystemMetrics: true.
	if _, ok := metrics["sys_os"]; !ok {
		t.Errorf("snapshot missing sys_os (CollectSystemMetrics should be on)")
	}

	// No forbidden keys in the wire payload.
	for k := range metrics {
		for _, banned := range forbiddenSubstrings {
			if strings.Contains(k, banned) {
				t.Errorf("snapshot contains forbidden key substring %q in key %q", banned, k)
			}
		}
	}
}

// forbiddenSubstrings mirrors the negative-space contract from
// data-model.md / contracts/telemetry-payload.md. Any of these appearing
// as a substring in a payload key is a privacy regression.
var forbiddenSubstrings = []string{
	"hostname", "fqdn",
	"container_name", "container_id",
	"endpoint_url", "endpoint_target", "target_host",
	"webhook_url", "webhook_target", "webhook_secret",
	"license_key", "holder_",
	"label_", "user_", "email",
}
