// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
)

// expectedKeys is the exact set of application-supplied keys per
// data-model.md Entity 1 layer A. Any deviation breaks the privacy
// contract review.
var expectedKeys = []string{
	"certificates_total",
	"containers_total",
	"edition",
	"endpoints_total",
	"heartbeats_total",
	"status_components_total",
	"webhooks_total",
}

func TestProvider_HappyPath_AllSevenKeys(t *testing.T) {
	logger, _ := setupTestLogger()
	deps := Deps{
		Containers:       fakeCounter{value: 42},
		Endpoints:        fakeCounter{value: 17},
		Heartbeats:       fakeCounter{value: 5},
		Certificates:    fakeCounter{value: 8},
		Webhooks:         fakeCounter{value: 2},
		StatusComponents: fakeCounter{value: 12},
		Edition:          staticEdition{value: extension.Enterprise},
	}
	got := newMetricsProvider(deps, logger)()

	gotKeys := keysOf(got)
	if !equalStringSlices(gotKeys, expectedKeys) {
		t.Fatalf("unexpected key set: got %v, want %v", gotKeys, expectedKeys)
	}

	if got["edition"] != "pro" {
		t.Errorf("edition: got %v, want \"pro\"", got["edition"])
	}
	if got["containers_total"] != 42 {
		t.Errorf("containers_total: got %v, want 42", got["containers_total"])
	}
	if got["endpoints_total"] != 17 {
		t.Errorf("endpoints_total: got %v, want 17", got["endpoints_total"])
	}
	if got["heartbeats_total"] != 5 {
		t.Errorf("heartbeats_total: got %v, want 5", got["heartbeats_total"])
	}
	if got["certificates_total"] != 8 {
		t.Errorf("certificates_total: got %v, want 8", got["certificates_total"])
	}
	if got["webhooks_total"] != 2 {
		t.Errorf("webhooks_total: got %v, want 2", got["webhooks_total"])
	}
	if got["status_components_total"] != 12 {
		t.Errorf("status_components_total: got %v, want 12", got["status_components_total"])
	}
}

func TestProvider_PartialFailure_OneCounterErrors(t *testing.T) {
	logger, buf := setupTestLogger()
	boom := errors.New("boom")
	deps := Deps{
		Containers:       fakeCounter{value: 100},
		Endpoints:        fakeCounter{err: boom}, // this one fails
		Heartbeats:       fakeCounter{value: 5},
		Certificates:    fakeCounter{value: 8},
		Webhooks:         fakeCounter{value: 2},
		StatusComponents: fakeCounter{value: 12},
		Edition:          staticEdition{value: extension.Community},
	}
	got := newMetricsProvider(deps, logger)()

	if got["endpoints_total"] != 0 {
		t.Errorf("failed counter should report 0, got %v", got["endpoints_total"])
	}
	// Other counters still populated — partial visibility per FR-009.
	if got["containers_total"] != 100 {
		t.Errorf("containers_total: got %v, want 100", got["containers_total"])
	}
	if got["edition"] != "community" {
		t.Errorf("edition: got %v, want \"community\"", got["edition"])
	}
	// And the failure is logged at WARN.
	if !strings.Contains(buf.String(), `"counter":"endpoints"`) {
		t.Errorf("expected WARN with counter=endpoints, got:\n%s", buf.String())
	}
}

func TestProvider_PanicContained(t *testing.T) {
	logger, buf := setupTestLogger()
	deps := Deps{
		Containers:       fakeCounter{panic: "oh no"},
		Endpoints:        fakeCounter{value: 1},
		Heartbeats:       fakeCounter{value: 1},
		Certificates:    fakeCounter{value: 1},
		Webhooks:         fakeCounter{value: 1},
		StatusComponents: fakeCounter{value: 1},
		Edition:          staticEdition{panic: "edition explodes"},
	}
	// The provider must NOT panic out — recover contains it (FR-012).
	got := newMetricsProvider(deps, logger)()

	if got["containers_total"] != 0 {
		t.Errorf("panicking counter should report 0, got %v", got["containers_total"])
	}
	if got["edition"] != "community" {
		t.Errorf("panicking edition source should fall back to community, got %v", got["edition"])
	}
	// Other counters unaffected.
	if got["endpoints_total"] != 1 {
		t.Errorf("endpoints_total: got %v, want 1", got["endpoints_total"])
	}
	logs := buf.String()
	if !strings.Contains(logs, `"counter":"containers"`) || !strings.Contains(logs, `"panic":"oh no"`) {
		t.Errorf("expected WARN with panic for containers counter, got:\n%s", logs)
	}
	if !strings.Contains(logs, `"panic":"edition explodes"`) {
		t.Errorf("expected WARN with panic for edition source, got:\n%s", logs)
	}
}

func TestProvider_NilCounter_ReportsZero(t *testing.T) {
	logger, _ := setupTestLogger()
	deps := Deps{
		// All nil — provider must not crash and must report 0s.
		Edition: staticEdition{value: extension.Community},
	}
	got := newMetricsProvider(deps, logger)()

	for _, key := range []string{
		"containers_total", "endpoints_total", "heartbeats_total",
		"certificates_total", "webhooks_total", "status_components_total",
	} {
		if got[key] != 0 {
			t.Errorf("%s with nil counter should be 0, got %v", key, got[key])
		}
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
