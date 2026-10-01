// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proberTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// fakeLabeledDiscoverer implements labeledDiscoverer for prober tests.
type fakeLabeledDiscoverer struct {
	mu      sync.Mutex
	results []*docker.DiscoveryResult
}

func (f *fakeLabeledDiscoverer) DiscoverAllWithLabels(_ context.Context) ([]*docker.DiscoveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.results, nil
}

func (f *fakeLabeledDiscoverer) set(results []*docker.DiscoveryResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = results
}

type probeLog struct {
	mu     sync.Mutex
	events []*agentpb.AgentEvent
}

func (l *probeLog) send(ev *agentpb.AgentEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
	return nil
}

func (l *probeLog) count(target string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, ev := range l.events {
		if ev.GetEndpoint().GetUrl() == target {
			n++
		}
	}
	return n
}

func (l *probeLog) all() []*agentpb.AgentEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*agentpb.AgentEvent(nil), l.events...)
}

// startProbes runs the endpoint prober until the test ends, with intervals short enough for a test.
func startProbes(t *testing.T, ld labeledDiscoverer, discoverEvery time.Duration) *probeLog {
	t.Helper()
	floor := endpoint.MinInterval
	endpoint.MinInterval = 10 * time.Millisecond
	t.Cleanup(func() { endpoint.MinInterval = floor })
	log := &probeLog{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runEndpointProbes(ctx, "agent-1", ld, log.send, discoverEvery, proberTestLogger())
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return log
}

func countingServer(t *testing.T, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestEndpointEvent(t *testing.T) {
	code := 200
	up := endpointEvent("agent-1", "http://x", endpoint.CheckResult{Success: true, ResponseTimeMs: 42, HTTPStatus: &code})

	ep := up.GetEndpoint()
	require.NotNil(t, ep)
	assert.Equal(t, "agent-1", up.GetAgentId())
	assert.Equal(t, "http://x", ep.GetUrl())
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_UP, ep.GetStatus())
	assert.Equal(t, uint32(200), ep.GetStatusCode())
	assert.Equal(t, uint64(42), ep.GetLatencyMs())
	assert.NotEmpty(t, up.GetEventId())

	down := endpointEvent("agent-1", "tcp://y:1", endpoint.CheckResult{Success: false, ErrorMessage: "connection refused"})
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN, down.GetEndpoint().GetStatus())
	assert.Equal(t, "connection refused", down.GetEndpoint().GetErrorMessage())
}

func TestCertEvent(t *testing.T) {
	nb := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	na := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	ev := certEvent("agent-1", "example.com", 8443, &certificate.CheckCertificateResult{
		SubjectCN: "example.com",
		IssuerCN:  "R3",
		SANs:      []string{"example.com", "www.example.com"},
		NotBefore: nb,
		NotAfter:  na,
	})

	ci := ev.GetCertificate()
	require.NotNil(t, ci)
	assert.Equal(t, "example.com", ci.GetHost())
	assert.Equal(t, uint32(8443), ci.GetPort())
	assert.Equal(t, "example.com", ci.GetSubjectCn())
	assert.Equal(t, "R3", ci.GetIssuerCn())
	assert.Equal(t, []string{"example.com", "www.example.com"}, ci.GetSanDns())
	assert.Equal(t, na.Unix(), ci.GetNotAfter().AsTime().Unix())
	assert.Equal(t, nb.Unix(), ci.GetNotBefore().AsTime().Unix())
}

func TestCertEvent_ZeroDatesOmitted(t *testing.T) {
	ev := certEvent("a", "h", 443, &certificate.CheckCertificateResult{SubjectCN: "h"})
	ci := ev.GetCertificate()
	assert.Nil(t, ci.GetNotBefore(), "zero NotBefore must not be set")
	assert.Nil(t, ci.GetNotAfter(), "zero NotAfter must not be set")
}

func TestRunEndpointProbes_HTTP(t *testing.T) {
	srv, _ := countingServer(t, 0)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
	}}

	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(srv.URL) == 1 }, 5*time.Second, 20*time.Millisecond)
	got := log.all()[0]
	ep := got.GetEndpoint()
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_UP, ep.GetStatus())
	assert.Equal(t, uint32(200), ep.GetStatusCode())
	assert.Equal(t, "agent-1", got.GetAgentId())
}

func TestRunEndpointProbes_DedupsTargetAcrossContainers(t *testing.T) {
	srv, hits := countingServer(t, 0)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
	}}

	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(srv.URL) >= 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 1, log.count(srv.URL), "the same target across containers must be probed once")
	assert.Equal(t, int32(1), hits.Load())
}

func TestRunEndpointProbes_NoLabelsNoEvents(t *testing.T) {
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"unrelated.label": "x"}},
	}}

	log := startProbes(t, ld, 20*time.Millisecond)

	time.Sleep(150 * time.Millisecond)
	assert.Empty(t, log.all(), "containers without endpoint labels must not produce events")
}

func TestRunEndpointProbes_EachEndpointKeepsItsOwnInterval(t *testing.T) {
	fast, _ := countingServer(t, 0)
	slow, _ := countingServer(t, 0)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{
			"maintenant.endpoint.0.http":     fast.URL,
			"maintenant.endpoint.0.interval": "150ms",
			"maintenant.endpoint.1.http":     slow.URL,
		}},
	}}

	log := startProbes(t, ld, time.Hour)

	time.Sleep(1200 * time.Millisecond)
	assert.GreaterOrEqual(t, log.count(fast.URL), 4, "an endpoint labelled every 150ms must not wait for a fixed 30s cadence")
	assert.Equal(t, 1, log.count(slow.URL), "an endpoint on the default interval is probed once, then every 30s")
}

func TestRunEndpointProbes_HonoursLabelTimeout(t *testing.T) {
	srv, _ := countingServer(t, 3*time.Second)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{
			"maintenant.endpoint.http":    srv.URL,
			"maintenant.endpoint.timeout": "300ms",
		}},
	}}

	start := time.Now()
	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(srv.URL) == 1 }, 2*time.Second, 20*time.Millisecond,
		"a 300ms timeout must end the probe long before the server answers")
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN, log.all()[0].GetEndpoint().GetStatus())
}

func TestRunEndpointProbes_FollowsLabelChanges(t *testing.T) {
	kept, _ := countingServer(t, 0)
	dropped, _ := countingServer(t, 0)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": kept.URL, "maintenant.endpoint.interval": "50ms"}},
		{Labels: map[string]string{"maintenant.endpoint.http": dropped.URL, "maintenant.endpoint.interval": "50ms"}},
	}}

	log := startProbes(t, ld, 50*time.Millisecond)
	require.Eventually(t, func() bool { return log.count(dropped.URL) >= 2 }, 5*time.Second, 20*time.Millisecond)

	ld.set([]*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": kept.URL, "maintenant.endpoint.interval": "50ms"}},
	})
	time.Sleep(200 * time.Millisecond)
	before := log.count(dropped.URL)
	keptBefore := log.count(kept.URL)
	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, before, log.count(dropped.URL), "a target whose label is gone must stop being probed")
	assert.Greater(t, log.count(kept.URL), keptBefore, "an unchanged target keeps its probe")
}
