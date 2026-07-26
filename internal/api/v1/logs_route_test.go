// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/agentserver"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// stubLogRequester stands in for agentserver.Sessions.
type stubLogRequester struct {
	connected bool
	capable   bool
	lines     []string
	err       error

	gotAgentID    string
	gotExternalID string
	gotLines      int
}

func (s *stubLogRequester) IsConnected(string) bool        { return s.connected }
func (s *stubLogRequester) HasCapability(_, _ string) bool { return s.capable }

func (s *stubLogRequester) FetchLogs(_ context.Context, agentID, externalID string, lines int, _ bool) ([]string, error) {
	s.gotAgentID, s.gotExternalID, s.gotLines = agentID, externalID, lines
	return s.lines, s.err
}

func (s *stubLogRequester) SendCommand(context.Context, string, string, *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error) {
	return nil, nil, s.err
}

func TestIsRemoteAgent(t *testing.T) {
	assert.False(t, isRemoteAgent(""), "an unattributed container is local")
	assert.False(t, isRemoteAgent(uid.LocalAgent), "the sentinel is the server's own runtime")
	assert.True(t, isRemoteAgent("11111111-2222-3333-4444-555555555555"))
}

// The old code answered every remote-container failure with "Cannot retrieve logs
// from Docker", which pointed operators at the wrong machine entirely.
func TestWriteRemoteLogsError_Taxonomy(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{"offline", agentserver.ErrAgentNotConnected, http.StatusServiceUnavailable, "AGENT_OFFLINE"},
		{"too old", agentserver.ErrAgentCannotServe, http.StatusNotImplemented, "AGENT_TOO_OLD"},
		{"busy", agentserver.ErrTooManyRequests, http.StatusTooManyRequests, "LOGS_BUSY"},
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout, "LOGS_TIMEOUT"},
		{"other", errors.New("boom"), http.StatusBadGateway, "LOGS_UNAVAILABLE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeRemoteLogsError(rec, "proxy2", tc.err)

			assert.Equal(t, tc.wantCode, rec.Code)

			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.wantBody, body.Error.Code)
			assert.Contains(t, body.Error.Message, "proxy2",
				"the message must name the agent so the operator knows which host to look at")
		})
	}
}

func TestFetchRemoteLogs_PassesThroughToAgent(t *testing.T) {
	req := &stubLogRequester{connected: true, capable: true, lines: []string{"a", "b"}}

	lines, err := fetchRemoteLogs(context.Background(), req, "agent-9", "ctr-abc", 250, true)

	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, lines)
	assert.Equal(t, "agent-9", req.gotAgentID)
	assert.Equal(t, "ctr-abc", req.gotExternalID, "the agent addresses the container by its runtime id")
	assert.Equal(t, 250, req.gotLines)
}

func TestFetchRemoteLogs_SurfacesAgentError(t *testing.T) {
	req := &stubLogRequester{connected: true, capable: true, err: agentserver.ErrAgentCannotServe}

	_, err := fetchRemoteLogs(context.Background(), req, "agent-9", "ctr", 100, false)

	assert.ErrorIs(t, err, agentserver.ErrAgentCannotServe)
}

// countingLogFetcher records whether the local runtime path was taken.
type countingLogFetcher struct {
	calls int
	lines []string
}

func (f *countingLogFetcher) FetchLogs(context.Context, string, int, bool) ([]string, error) {
	f.calls++
	return f.lines, nil
}

// logsHandler builds a ContainerHandler over a single seeded container.
func logsHandler(t *testing.T, c *container.Container, local *countingLogFetcher, remote *stubLogRequester) *ContainerHandler {
	t.Helper()
	store := &oneContainerStore{c: c}
	svc := container.NewService(container.Deps{
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	h := NewContainerHandler(svc, nil)
	if local != nil {
		h.SetLogFetcher(local)
	}
	if remote != nil {
		h.SetLogRequester(remote)
	}
	return h
}

func doLogsRequest(h *ContainerHandler, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/"+id+"/logs", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleLogs(rec, req)
	return rec
}

// Issue #39: a container on a remote agent must be served by that agent. The old
// code asked the server's own Docker daemon, which has never heard of it.
func TestHandleLogs_RemoteContainerGoesToAgent(t *testing.T) {
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "deadbeef", Name: "web",
		AgentID: "11111111-2222-3333-4444-555555555555",
	}
	local := &countingLogFetcher{lines: []string{"WRONG"}}
	remote := &stubLogRequester{connected: true, capable: true, lines: []string{"from-agent"}}

	rec := doLogsRequest(logsHandler(t, c, local, remote), c.ID)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, local.calls, "the local runtime must never be asked about a remote container")
	assert.Equal(t, "deadbeef", remote.gotExternalID)

	var body struct {
		Lines []string `json:"lines"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, []string{"from-agent"}, body.Lines)
}

func TestHandleLogs_LocalContainerStaysOnLocalRuntime(t *testing.T) {
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "cafe", Name: "db", AgentID: uid.LocalAgent,
	}
	local := &countingLogFetcher{lines: []string{"from-local"}}
	remote := &stubLogRequester{connected: true, capable: true, lines: []string{"WRONG"}}

	rec := doLogsRequest(logsHandler(t, c, local, remote), c.ID)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, local.calls)
	assert.Empty(t, remote.gotExternalID, "the agent must not be consulted for a local container")
}

func TestHandleLogs_OfflineAgentReportsItPlainly(t *testing.T) {
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "dead", Name: "web",
		AgentID: "11111111-2222-3333-4444-555555555555",
	}
	remote := &stubLogRequester{err: agentserver.ErrAgentNotConnected}

	rec := doLogsRequest(logsHandler(t, c, &countingLogFetcher{}, remote), c.ID)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "AGENT_OFFLINE", body.Error.Code)
}

func TestHandleLogs_RemoteWithoutAgentSupportIsExplicit(t *testing.T) {
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "dead", Name: "web",
		AgentID: "11111111-2222-3333-4444-555555555555",
	}

	rec := doLogsRequest(logsHandler(t, c, &countingLogFetcher{}, nil), c.ID)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "RUNTIME_UNAVAILABLE", body.Error.Code)
}

// oneContainerStore is the smallest ContainerStore that can back GetContainer.
type oneContainerStore struct {
	container.ContainerStore
	c *container.Container
}

func (s *oneContainerStore) GetContainerByID(_ context.Context, id string) (*container.Container, error) {
	if s.c != nil && s.c.ID == id {
		clone := *s.c
		return &clone, nil
	}
	return nil, nil
}

// stubAgentDirectory is declared in containers_agent_test.go (same package).
func TestResolveAgentLabel(t *testing.T) {
	const id = "11111111-2222-3333-4444-555555555555"

	t.Run("prefers the label", func(t *testing.T) {
		dir := stubAgentDirectory{names: map[string]AgentName{id: {Hostname: "host-a", Label: "proxy2"}}}
		assert.Equal(t, "proxy2", resolveAgentLabel(context.Background(), dir, id))
	})

	t.Run("falls back to the hostname", func(t *testing.T) {
		dir := stubAgentDirectory{names: map[string]AgentName{id: {Hostname: "host-a"}}}
		assert.Equal(t, "host-a", resolveAgentLabel(context.Background(), dir, id))
	})

	t.Run("falls back to the id when unknown", func(t *testing.T) {
		dir := stubAgentDirectory{names: map[string]AgentName{}}
		assert.Equal(t, id, resolveAgentLabel(context.Background(), dir, id))
	})

	t.Run("falls back to the id on lookup failure", func(t *testing.T) {
		dir := stubAgentDirectory{err: errors.New("db down")}
		assert.Equal(t, id, resolveAgentLabel(context.Background(), dir, id))
	})

	t.Run("falls back to the id with no directory", func(t *testing.T) {
		assert.Equal(t, id, resolveAgentLabel(context.Background(), nil, id))
	})
}

// An offline agent's error must name the agent the operator knows, not its UUID.
func TestHandleLogs_OfflineAgentErrorUsesLabel(t *testing.T) {
	const agentID = "11111111-2222-3333-4444-555555555555"
	c := &container.Container{ID: "ctr-uuid", ExternalID: "dead", Name: "web", AgentID: agentID}

	h := logsHandler(t, c, &countingLogFetcher{}, &stubLogRequester{err: agentserver.ErrAgentNotConnected})
	h.SetAgentDirectory(stubAgentDirectory{names: map[string]AgentName{agentID: {Label: "proxy2"}}})

	rec := doLogsRequest(h, c.ID)

	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Error.Message, "proxy2")
	assert.NotContains(t, body.Error.Message, agentID, "the raw UUID is meaningless to an operator")
}
