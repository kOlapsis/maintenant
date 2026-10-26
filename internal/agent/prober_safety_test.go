// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, US2 scenario 6, FR-014: the agent applies the
// same bounds as the server to the monitors it discovers by label on its host.

type capturedRequest struct {
	method  string
	headers http.Header
}

func capturingServer(t *testing.T) (*httptest.Server, string, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, capturedRequest{method: r.Method, headers: r.Header.Clone()})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)
	return srv, port, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), reqs...)
	}
}

func firstEndpointEvent(log *probeLog, target string) *agentpb.EndpointEvent {
	for _, ev := range log.all() {
		if ev.GetEndpoint().GetUrl() == target {
			return ev.GetEndpoint()
		}
	}
	return nil
}

// FR-010, FR-011 on the agent: the probe goes out as GET without the Authorization header.
func TestRunEndpointProbes_LabelMethodAndHeadersAreBounded(t *testing.T) {
	srv, _, requests := capturingServer(t)
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{
			"maintenant.endpoint.http":         srv.URL,
			"maintenant.endpoint.http.method":  "DELETE",
			"maintenant.endpoint.http.headers": `{"Authorization":"Bearer x","X-Trace":"1"}`,
		}},
	}}

	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(srv.URL) == 1 }, 5*time.Second, 20*time.Millisecond)
	reqs := requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].method, "a label may not make the agent send DELETE")
	assert.Empty(t, reqs[0].headers.Get("Authorization"))
	assert.Equal(t, "1", reqs[0].headers.Get("X-Trace"))
}

// FR-012, FR-014, SC-003: a label-declared monitor on the agent never connects to an unspecified address.
func TestRunEndpointProbes_LabelDestinationIsGuarded(t *testing.T) {
	_, port, requests := capturingServer(t)
	target := "http://0.0.0.0:" + port + "/"
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": target, "maintenant.endpoint.timeout": "2s"}},
	}}

	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(target) == 1 }, 5*time.Second, 20*time.Millisecond)
	ev := firstEndpointEvent(log, target)
	require.NotNil(t, ev)
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN, ev.GetStatus())
	assert.Contains(t, ev.GetErrorMessage(), "probe guard: blocked connection to 0.0.0.0")
	assert.Empty(t, requests(), "no request may reach the server")
}

// FR-012 on the agent, TCP form.
func TestRunEndpointProbes_LabelTCPDestinationIsGuarded(t *testing.T) {
	_, port, _ := capturingServer(t)
	target := "0.0.0.0:" + port
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.tcp": target}},
	}}

	log := startProbes(t, ld, time.Hour)

	require.Eventually(t, func() bool { return log.count(target) == 1 }, 5*time.Second, 20*time.Millisecond)
	ev := firstEndpointEvent(log, target)
	require.NotNil(t, ev)
	assert.Equal(t, agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN, ev.GetStatus())
	assert.Contains(t, ev.GetErrorMessage(), "probe guard: blocked connection to 0.0.0.0")
}

// FR-014: the endpoints the agent discovers carry the label source, which is what decides the bounds.
func TestDiscoverEndpoints_MarksTheLabelSource(t *testing.T) {
	ld := &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.endpoint.http": "http://api:3000/ready", "maintenant.endpoint.1.tcp": "db:5432"}},
	}}

	declared, err := discoverEndpoints(context.Background(), ld, proberTestLogger())
	require.NoError(t, err)
	require.Len(t, declared, 2)
	for target, ep := range declared {
		assert.Equal(t, endpoint.SourceLabel, ep.Source, target)
	}
}

// Edge case "moniteur de certificat issu de label", on the agent: the TLS scan is refused towards an
// unspecified address and still runs towards loopback.
func TestScanCertsOnce_LabelDestinationIsGuarded(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	require.NoError(t, err)

	guarded := &probeLog{}
	err = scanCertsOnce(context.Background(), "agent-1", &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.tls.certificates": "0.0.0.0:" + port}},
	}}, guarded.send, proberTestLogger())
	require.NoError(t, err)
	assert.Empty(t, guarded.all(), "a refused scan pushes no certificate")

	open := &probeLog{}
	err = scanCertsOnce(context.Background(), "agent-1", &fakeLabeledDiscoverer{results: []*docker.DiscoveryResult{
		{Labels: map[string]string{"maintenant.tls.certificates": "127.0.0.1:" + port}},
	}}, open.send, proberTestLogger())
	require.NoError(t, err)
	require.Len(t, open.all(), 1, "a loopback target is still scanned")
	assert.Equal(t, "127.0.0.1", open.all()[0].GetCertificate().GetHost())
}
