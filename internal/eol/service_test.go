// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package eol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/hoststat"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeStore struct {
	mu       sync.Mutex
	agents   []agent.Agent
	osWrites []agent.OSIdentity
}

func (f *fakeStore) ListAgentsForEOL(context.Context) ([]agent.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]agent.Agent, len(f.agents))
	copy(out, f.agents)
	return out, nil
}

func (f *fakeStore) UpdateAgentOS(_ context.Context, _ string, os agent.OSIdentity, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.osWrites = append(f.osWrites, os)
	return true, nil
}

func (f *fakeStore) set(agents ...agent.Agent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents = agents
}

type collector struct {
	mu     sync.Mutex
	events []alert.Event
}

func (c *collector) emit(evt alert.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, evt)
}

func (c *collector) drain() []alert.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

func debianAgent(id, version, pretty string) agent.Agent {
	return agent.Agent{
		AgentID:         id,
		Hostname:        "host-" + id,
		Label:           id,
		DetectedRuntime: "docker",
		OSID:            "debian",
		OSVersionID:     version,
		OSPrettyName:    pretty,
		OSSource:        hoststat.OSSourceHostFile,
	}
}

func newTestService(t *testing.T, store AgentStore, emit func(alert.Event), now *time.Time) *Service {
	t.Helper()
	svc, err := New(Deps{
		Store:       store,
		Emit:        emit,
		ReadLocalOS: func() hoststat.OSRelease { return hoststat.OSRelease{} },
		Now:         func() time.Time { return *now },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestEvaluateAllDebianJourney(t *testing.T) {
	store := &fakeStore{}
	store.set(debianAgent("web-1", "11", "Debian GNU/Linux 11 (bullseye)"))
	var events collector
	now := time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got := events.drain()
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	warning := got[0]
	if warning.Severity != alert.SeverityWarning || warning.IsRecover {
		t.Fatalf("warning = %+v", warning)
	}
	if warning.Source != alert.SourceHost || warning.AlertType != AlertType ||
		warning.EntityType != "agent" || warning.EntityID != "web-1" || warning.EntityName != "web-1" {
		t.Fatalf("event key = %+v", warning)
	}
	if want := "Debian GNU/Linux 11 (bullseye): free security support ends on 2026-08-31 (in 30 days)"; warning.Message != want {
		t.Fatalf("message = %q, want %q", warning.Message, want)
	}
	if warning.Details["state"] != string(StateEndingSoon) || warning.Details["product"] != "debian" ||
		warning.Details["cycle"] != "11" || warning.Details["days_remaining"] != 30 ||
		warning.Details["security_until"] != "2026-08-31" || warning.Details["table_source"] != SourceEmbedded {
		t.Fatalf("details = %#v", warning.Details)
	}
	if warning.Timestamp != now {
		t.Fatalf("timestamp = %v, want %v", warning.Timestamp, now)
	}

	now = time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got = events.drain()
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	critical := got[0]
	if critical.Severity != alert.SeverityCritical || critical.IsRecover {
		t.Fatalf("critical = %+v", critical)
	}
	if want := "Debian GNU/Linux 11 (bullseye): free security support ended on 2026-08-31 (1 day ago)"; critical.Message != want {
		t.Fatalf("message = %q, want %q", critical.Message, want)
	}

	store.set(debianAgent("web-1", "12", "Debian GNU/Linux 12 (bookworm)"))
	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got = events.drain()
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	if !got[0].IsRecover || got[0].Severity != alert.SeverityInfo {
		t.Fatalf("recovery = %+v", got[0])
	}
	if got[0].EntityID != "web-1" || got[0].AlertType != AlertType {
		t.Fatalf("recovery key = %+v", got[0])
	}
}

func TestEvaluateNeverAlertsWithoutAKnownCycle(t *testing.T) {
	unknown := agent.Agent{
		AgentID:             "no-os",
		Hostname:            "host-no-os",
		OSSource:            hoststat.OSSourceHostFile,
		OSUnavailableReason: hoststat.OSReasonMountMissing,
	}
	untracked := agent.Agent{
		AgentID:      "fedora",
		Hostname:     "host-fedora",
		OSID:         "fedora",
		OSVersionID:  "42",
		OSPrettyName: "Fedora Linux 42",
		OSSource:     hoststat.OSSourceHostFile,
	}
	store := &fakeStore{}
	store.set(unknown, untracked)
	var events collector
	now := time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got := events.drain()
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	for _, evt := range got {
		if !evt.IsRecover || evt.Severity != alert.SeverityInfo {
			t.Fatalf("event = %+v, want a recovery", evt)
		}
	}
	if got[0].Message != "Host operating system not reported" {
		t.Fatalf("unknown message = %q", got[0].Message)
	}
	if got[1].Message != "Fedora Linux 42: end of support is not tracked" {
		t.Fatalf("untracked message = %q", got[1].Message)
	}
}

func TestSeverityDropResolvesBeforeWarning(t *testing.T) {
	store := &fakeStore{}
	store.set(debianAgent("web-1", "11", "Debian 11"))
	var events collector
	now := time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)
	svc.SeedSeverities([]alert.Alert{
		{Source: alert.SourceHost, AlertType: AlertType, Severity: alert.SeverityCritical, EntityID: "web-1"},
		{Source: alert.SourceContainer, AlertType: "restart_loop", Severity: alert.SeverityCritical, EntityID: "other"},
	})

	server := fixtureServer(t, map[string]http.HandlerFunc{
		"debian": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"releases":[
				{"name":"11","isLts":false,"eoasFrom":"2024-08-14","eolFrom":"2026-09-21","eoesFrom":"2031-06-30"}
			]}}`))
		},
	})
	svc.fetcher = &Fetcher{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return now }}
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got := events.drain()
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if !got[0].IsRecover || got[0].Severity != alert.SeverityInfo {
		t.Fatalf("first event = %+v, want a recovery", got[0])
	}
	if got[1].IsRecover || got[1].Severity != alert.SeverityWarning {
		t.Fatalf("second event = %+v, want a warning", got[1])
	}
	if want := "Debian 11: free security support ends on 2026-09-21 (in 20 days)"; got[1].Message != want {
		t.Fatalf("message = %q, want %q", got[1].Message, want)
	}
	if got[1].Details["table_source"] != SourceRemote {
		t.Fatalf("table_source = %v", got[1].Details["table_source"])
	}
}

func TestEvaluateAllResolvesAgentsThatLeft(t *testing.T) {
	store := &fakeStore{}
	store.set(debianAgent("web-1", "11", "Debian 11"))
	var events collector
	now := time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	events.drain()

	store.set()
	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	got := events.drain()
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	if !got[0].IsRecover || got[0].EntityID != "web-1" {
		t.Fatalf("event = %+v", got[0])
	}

	if err := svc.EvaluateAll(context.Background()); err != nil {
		t.Fatalf("EvaluateAll: %v", err)
	}
	if got := events.drain(); len(got) != 0 {
		t.Fatalf("events = %d, want none", len(got))
	}
}

func TestEvaluateAgentOnly(t *testing.T) {
	store := &fakeStore{}
	store.set(debianAgent("web-1", "11", "Debian 11"), debianAgent("web-2", "13", "Debian 13"))
	var events collector
	now := time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	if err := svc.EvaluateAgent(context.Background(), "web-1"); err != nil {
		t.Fatalf("EvaluateAgent: %v", err)
	}
	got := events.drain()
	if len(got) != 1 || got[0].EntityID != "web-1" || got[0].Severity != alert.SeverityCritical {
		t.Fatalf("events = %+v", got)
	}
}

func TestRefreshKeepsTheEmbeddedTableOnFailure(t *testing.T) {
	store := &fakeStore{}
	var events collector
	now := time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	server := fixtureServer(t, map[string]http.HandlerFunc{
		"sles": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
	})
	svc.fetcher = &Fetcher{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return now }}

	if err := svc.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh: want an error")
	}
	status := svc.Status()
	if status.Source != SourceEmbedded {
		t.Fatalf("source = %q, want %q", status.Source, SourceEmbedded)
	}
	if status.LastRefreshError == "" {
		t.Fatal("LastRefreshError is empty")
	}
	if !status.RefreshEnabled {
		t.Fatal("RefreshEnabled = false")
	}
}

func TestRefreshAppliesTheRemoteTable(t *testing.T) {
	store := &fakeStore{}
	var events collector
	now := time.Date(2026, time.September, 18, 0, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	server := fixtureServer(t, nil)
	svc.fetcher = &Fetcher{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return now }}

	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	status := svc.Status()
	if status.Source != SourceRemote {
		t.Fatalf("source = %q, want %q", status.Source, SourceRemote)
	}
	if status.LastRefreshError != "" {
		t.Fatalf("LastRefreshError = %q", status.LastRefreshError)
	}
	if !status.FetchedAt.Equal(now) {
		t.Fatalf("fetched_at = %v, want %v", status.FetchedAt, now)
	}
}

func TestStartWithoutFetcherEvaluatesWithoutFetching(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	store := &fakeStore{}
	store.set(debianAgent("web-1", "11", "Debian 11"))
	var events collector
	now := time.Date(2026, time.September, 1, 6, 0, 0, 0, time.UTC)
	svc, err := New(Deps{
		Store: store,
		Emit:  events.emit,
		Now:   func() time.Time { return now },
		ReadLocalOS: func() hoststat.OSRelease {
			return hoststat.OSRelease{ID: "debian", VersionID: "13", PrettyName: "Debian 13", Source: hoststat.OSSourceHostFile}
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Status().RefreshEnabled {
		t.Fatal("RefreshEnabled = true without a fetcher")
	}

	restore := startDelay
	startDelay = time.Millisecond
	t.Cleanup(func() { startDelay = restore })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	deadline := time.After(2 * time.Second)
	for {
		store.mu.Lock()
		writes := len(store.osWrites)
		store.mu.Unlock()
		if writes > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no local os write")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Start: %v", err)
	}

	if requests.Load() != 0 {
		t.Fatalf("http requests = %d, want 0", requests.Load())
	}
	store.mu.Lock()
	wrote := store.osWrites[0]
	store.mu.Unlock()
	if wrote.ID != "debian" || wrote.VersionID != "13" || wrote.Source != hoststat.OSSourceHostFile {
		t.Fatalf("local os = %+v", wrote)
	}
	got := events.drain()
	if len(got) == 0 || got[0].Severity != alert.SeverityCritical {
		t.Fatalf("events = %+v, want a critical", got)
	}
}

func TestHostSupportsOrdersWorstFirst(t *testing.T) {
	store := &fakeStore{}
	store.set(
		debianAgent("supported", "13", "Debian 13"),
		debianAgent("security-only", "12", "Debian 12"),
		agent.Agent{AgentID: "too-old", Hostname: "host-too-old", Label: "too-old"},
		debianAgent("ended-recent", "10", "Debian 10"),
		agent.Agent{AgentID: "untracked", Label: "untracked", OSID: "fedora", OSVersionID: "42", OSSource: hoststat.OSSourceHostFile},
		debianAgent("ending-soon", "11", "Debian 11"),
		debianAgent("ended-old", "9", "Debian 9"),
		agent.Agent{AgentID: "no-os", Label: "no-os", OSSource: hoststat.OSSourceHostFile, OSUnavailableReason: hoststat.OSReasonMountMissing},
		agent.Agent{AgentID: uid.LocalAgent, Hostname: "local", Label: "local", OSID: "debian", OSVersionID: "13", OSSource: hoststat.OSSourceHostFile},
	)
	var events collector
	now := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)
	svc := newTestService(t, store, events.emit, &now)

	hosts, err := svc.HostSupports(context.Background())
	if err != nil {
		t.Fatalf("HostSupports: %v", err)
	}
	want := []string{"ended-old", "ended-recent", "ending-soon", "no-os", "too-old", "untracked", "security-only", "local", "supported"}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %d, want %d", len(hosts), len(want))
	}
	for i, label := range want {
		if hosts[i].Label != label {
			t.Fatalf("host %d = %q, want %q", i, hosts[i].Label, label)
		}
	}

	byLabel := make(map[string]HostSupport, len(hosts))
	for _, h := range hosts {
		byLabel[h.Label] = h
	}
	if got := byLabel["too-old"].Identity.UnavailableReason; got != ReasonAgentTooOld {
		t.Fatalf("too-old reason = %q, want %q", got, ReasonAgentTooOld)
	}
	if got := byLabel["too-old"].Support.State; got != StateUnknown {
		t.Fatalf("too-old state = %q", got)
	}
	if got := byLabel["no-os"].Identity.UnavailableReason; got != hoststat.OSReasonMountMissing {
		t.Fatalf("no-os reason = %q", got)
	}
	local := byLabel["local"]
	if !local.IsLocal {
		t.Fatal("local host not flagged")
	}
	if got := byLabel["security-only"].Support.State; got != StateSecurityOnly {
		t.Fatalf("security-only state = %q", got)
	}

	counts := CountStates(hosts)
	wantCounts := map[string]int{"ended": 2, "ending_soon": 1, "unknown": 2, "untracked": 1, "supported": 3}
	for state, n := range wantCounts {
		if counts[state] != n {
			t.Fatalf("counts[%s] = %d, want %d", state, counts[state], n)
		}
	}
}
