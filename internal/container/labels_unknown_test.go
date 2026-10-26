// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

// Acceptance tests of 047-audit-small-fixes, C13: a typo in a maintenant.* label is reported, once per container.

type logSink struct {
	mu sync.Mutex
	sb strings.Builder
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.Write(p)
}

func (l *logSink) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.String()
}

func captureLogs() (*slog.Logger, *logSink) {
	sink := &logSink{}
	return slog.New(slog.NewTextHandler(sink, &slog.HandlerOptions{Level: slog.LevelDebug})), sink
}

// warnLines returns the WARN log lines mentioning needle.
func warnLines(logs, needle string) []string {
	var out []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, needle) {
			out = append(out, line)
		}
	}
	return out
}

// FR-010, US4 scenario 2: an unknown label is warned about once per container, and the warning names it.
func TestWarnUnknownLabels_ReportsOncePerContainer(t *testing.T) {
	logger, logs := captureLogs()
	labels := map[string]string{
		"maintenant.alert.restart-threshold": "3",
		"com.docker.compose.project":         "demo",
	}

	for i := 0; i < 3; i++ {
		WarnUnknownLabels(extID("typo-a"), labels, logger)
	}
	lines := warnLines(logs.String(), "maintenant.alert.restart-threshold")
	require.Len(t, lines, 1, "one warning per container, got:\n%s", logs.String())
	assert.Contains(t, lines[0], extID("typo-a")[:12], "the warning names the container")
	assert.NotContains(t, logs.String(), "com.docker.compose.project", "only maintenant.* labels are checked")

	WarnUnknownLabels(extID("typo-b"), labels, logger)
	assert.Len(t, warnLines(logs.String(), "maintenant.alert.restart-threshold"), 2, "another container gets its own warning")
}

// FR-010: each unknown label of a container is named, once.
func TestWarnUnknownLabels_NamesEveryUnknownLabel(t *testing.T) {
	logger, logs := captureLogs()
	labels := map[string]string{
		"maintenant.alrt.severity":  "critical",
		"maintenant.groupe":         "web",
		"maintenant.endpoints.http": "http://web/health",
		"maintenant.group":          "web",
	}

	WarnUnknownLabels(extID("typo-c"), labels, logger)
	WarnUnknownLabels(extID("typo-c"), labels, logger)

	for _, unknown := range []string{"maintenant.alrt.severity", "maintenant.groupe", "maintenant.endpoints.http"} {
		assert.Len(t, warnLines(logs.String(), unknown), 1, "%s must be warned about once", unknown)
	}
	assert.Empty(t, warnLines(logs.String(), "maintenant.group "), "maintenant.group is known")
	assert.Empty(t, warnLines(logs.String(), "label=maintenant.group\n"))
}

// FR-010, edge case "label connu mais mal typé": known keys and known prefixes raise no "unknown" warning.
func TestWarnUnknownLabels_KeepsQuietOnKnownLabels(t *testing.T) {
	logger, logs := captureLogs()
	labels := map[string]string{
		"maintenant.ignore":                  "false",
		"maintenant.group":                   "web",
		"maintenant.alert.severity":          "critical",
		"maintenant.alert.restart_threshold": "abc",
		"maintenant.endpoint.http":           "http://web:8080/health",
		"maintenant.endpoint.interval":       "30s",
		"maintenant.endpoint.0.tcp":          "db:5432",
		"maintenant.endpoint.0.http.method":  "HEAD",
		"maintenant.tls.certificates":        "example.com",
		"maintenant.proxy-labels":            "false",
		"maintenant.update.enabled":          "true",
		"maintenant.update.track":            "minor",
		"maintenant.update.tag-include":      "^v",
		"maintenant.update.pin":              "yes",
	}

	WarnUnknownLabels(extID("known-d"), labels, logger)

	assert.Empty(t, warnLines(logs.String(), "maintenant."), "no known label may be reported as unknown:\n%s", logs.String())
}

// FR-010, US4 scenario 2: a container reported by an agent is warned about once, however often it is reported.
func TestHandleAgentEvent_WarnsOnceAboutAnUnknownLabel(t *testing.T) {
	store := newSvcStore()
	logger, logs := captureLogs()
	svc := newTestService(store, func(d *Deps) { d.Logger = logger })
	id := extID("agent-typo")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "typo",
		Image:       "nginx:1.27",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels:      map[string]string{"maintenant.alert.restart-threshold": "3"},
	}

	for i := 0; i < 3; i++ {
		require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev, agentevent.Meta{ObservedAt: time.Now()}))
	}

	assert.Len(t, warnLines(logs.String(), "maintenant.alert.restart-threshold"), 1, "got:\n%s", logs.String())
}
