// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

// agentEnrichStore is a minimal ContainerStore returning a fixed list, used to
// drive the container list handler in tests.
type agentEnrichStore struct{ containers []*container.Container }

func (s *agentEnrichStore) ListContainers(_ context.Context, _ container.ListContainersOpts) ([]*container.Container, error) {
	return s.containers, nil
}
func (s *agentEnrichStore) InsertContainer(context.Context, *container.Container) (string, error) {
	return "", nil
}
func (s *agentEnrichStore) UpdateContainer(context.Context, *container.Container) error { return nil }
func (s *agentEnrichStore) GetContainerByExternalID(context.Context, string, string) (*container.Container, error) {
	return nil, nil
}
func (s *agentEnrichStore) GetContainerByID(_ context.Context, id string) (*container.Container, error) {
	for _, c := range s.containers {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (s *agentEnrichStore) ArchiveContainer(context.Context, string, time.Time) error { return nil }
func (s *agentEnrichStore) DeleteContainerByID(context.Context, string) error         { return nil }
func (s *agentEnrichStore) InsertTransition(context.Context, *container.StateTransition) (string, error) {
	return "", nil
}
func (s *agentEnrichStore) ListTransitionsByContainer(context.Context, string, container.ListTransitionsOpts) ([]*container.StateTransition, int, error) {
	return nil, 0, nil
}
func (s *agentEnrichStore) CountRestartsSince(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (s *agentEnrichStore) GetTransitionsInWindow(context.Context, string, time.Time, time.Time) ([]*container.StateTransition, error) {
	return nil, nil
}
func (s *agentEnrichStore) DeleteTransitionsBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
func (s *agentEnrichStore) DeleteArchivedContainersBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

type stubAgentDirectory struct {
	names map[string]AgentName
	err   error
}

func (s stubAgentDirectory) AgentNames(context.Context) (map[string]AgentName, error) {
	return s.names, s.err
}

func TestHandleList_EnrichesAgentIdentity(t *testing.T) {
	agentID := "agent-1"
	store := &agentEnrichStore{containers: []*container.Container{
		{ID: "1", ExternalID: "ext-local", Name: "local-app", State: container.StateRunning},
		{ID: "2", ExternalID: "ext-remote", Name: "remote-app", State: container.StateRunning, AgentID: agentID},
	}}
	svc := container.NewService(container.Deps{Store: store, Logger: slog.Default()})

	h := NewContainerHandler(svc, nil)
	h.SetAgentDirectory(stubAgentDirectory{names: map[string]AgentName{
		"agent-1": {Hostname: "edge-host", Label: "edge"},
	}})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers", h.HandleList)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Groups []struct {
			Containers []struct {
				Name          string  `json:"name"`
				AgentID       *string `json:"agent_id"`
				AgentHostname *string `json:"agent_hostname"`
				AgentLabel    *string `json:"agent_label"`
			} `json:"containers"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byName := map[string]struct {
		hostname *string
		label    *string
	}{}
	for _, g := range resp.Groups {
		for _, c := range g.Containers {
			byName[c.Name] = struct {
				hostname *string
				label    *string
			}{c.AgentHostname, c.AgentLabel}
		}
	}

	remote, ok := byName["remote-app"]
	require.True(t, ok, "remote-app should be present")
	require.NotNil(t, remote.hostname)
	assert.Equal(t, "edge-host", *remote.hostname)
	require.NotNil(t, remote.label)
	assert.Equal(t, "edge", *remote.label)

	local, ok := byName["local-app"]
	require.True(t, ok, "local-app should be present")
	assert.Nil(t, local.hostname, "local container must not carry agent identity")
	assert.Nil(t, local.label)
}

func TestHandleList_NoAgentDirectoryIsSafe(t *testing.T) {
	agentID := "agent-1"
	store := &agentEnrichStore{containers: []*container.Container{
		{ID: "2", ExternalID: "ext-remote", Name: "remote-app", State: container.StateRunning, AgentID: agentID},
	}}
	svc := container.NewService(container.Deps{Store: store, Logger: slog.Default()})
	h := NewContainerHandler(svc, nil) // no agent directory wired

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers", h.HandleList)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleGet_EnrichesAgentIdentity(t *testing.T) {
	store := &agentEnrichStore{containers: []*container.Container{
		{ID: "1", ExternalID: "ext-local", Name: "local-app", State: container.StateRunning},
		{ID: "2", ExternalID: "ext-remote", Name: "remote-app", State: container.StateRunning, AgentID: "agent-1"},
	}}
	svc := container.NewService(container.Deps{Store: store, Logger: slog.Default()})

	h := NewContainerHandler(svc, nil)
	h.SetAgentDirectory(stubAgentDirectory{names: map[string]AgentName{
		"agent-1": {Hostname: "edge-host", Label: "edge"},
	}})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers/{id}", h.HandleGet)

	get := func(id string) map[string]interface{} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/"+id, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body
	}

	remote := get("2")
	assert.Equal(t, "agent-1", remote["agent_id"])
	assert.Equal(t, "edge-host", remote["agent_hostname"])
	assert.Equal(t, "edge", remote["agent_label"])

	local := get("1")
	assert.NotContains(t, local, "agent_id", "local container must not carry agent identity")
	assert.NotContains(t, local, "agent_hostname")
	assert.NotContains(t, local, "agent_label")
}

func TestHandleGet_CarriesTheSwarmService(t *testing.T) {
	store := &agentEnrichStore{containers: []*container.Container{{
		ID: "1", ExternalID: "ext-task", Name: "prod_web.2.x7k", State: container.StateRunning,
		ControllerKind: container.ControllerSwarmService, SwarmServiceID: "svc1", SwarmServiceName: "prod_web",
		SwarmNodeID: "node1", SwarmTaskSlot: 2,
	}}}
	svc := container.NewService(container.Deps{Store: store, Logger: slog.Default()})
	h := NewContainerHandler(svc, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers/{id}", h.HandleGet)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "swarm-service", body["controller_kind"])
	assert.Equal(t, "svc1", body["swarm_service_id"])
	assert.Equal(t, "prod_web", body["swarm_service_name"])
	assert.Equal(t, "node1", body["swarm_node_id"])
	assert.EqualValues(t, 2, body["swarm_task_slot"])
}
