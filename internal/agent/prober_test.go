// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	results []*docker.DiscoveryResult
}

func (f *fakeLabeledDiscoverer) DiscoverAllWithLabels(_ context.Context) ([]*docker.DiscoveryResult, error) {
	return f.results, nil
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

func TestProbeEndpointsOnce_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
	}}

	var got []*agentpb.AgentEvent
	send := func(ev *agentpb.AgentEvent) error { got = append(got, ev); return nil }

	err := probeEndpointsOnce(context.Background(), "agent-1", ld, send, proberTestLogger())
	require.NoError(t, err)

	require.Len(t, got, 1)
	ep := got[0].GetEndpoint()
	require.NotNil(t, ep)
	assert.Equal(t, srv.URL, ep.GetUrl())
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_UP, ep.GetStatus())
	assert.Equal(t, uint32(200), ep.GetStatusCode())
	assert.Equal(t, "agent-1", got[0].GetAgentId())
}

func TestProbeEndpointsOnce_DedupsTargetAcrossContainers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Two containers declaring the same endpoint target → probed once.
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
		{Labels: map[string]string{"maintenant.endpoint.http": srv.URL}},
	}}

	var got []*agentpb.AgentEvent
	send := func(ev *agentpb.AgentEvent) error { got = append(got, ev); return nil }

	require.NoError(t, probeEndpointsOnce(context.Background(), "a", ld, send, proberTestLogger()))
	assert.Len(t, got, 1, "the same target across containers must be probed once")
}

func TestProbeEndpointsOnce_NoLabelsNoEvents(t *testing.T) {
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"unrelated.label": "x"}},
	}}
	sent := 0
	send := func(*agentpb.AgentEvent) error { sent++; return nil }

	require.NoError(t, probeEndpointsOnce(context.Background(), "a", ld, send, proberTestLogger()))
	assert.Zero(t, sent, "containers without endpoint labels must not produce events")
}
