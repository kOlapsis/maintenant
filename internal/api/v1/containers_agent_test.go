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
func (s *agentEnrichStore) GetContainerByExternalID(context.Context, string) (*container.Container, error) {
	return nil, nil
}
func (s *agentEnrichStore) GetContainerByID(context.Context, string) (*container.Container, error) {
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

type stubAgentDirectory struct{ names map[string]AgentName }

func (s stubAgentDirectory) AgentNames(context.Context) (map[string]AgentName, error) {
	return s.names, nil
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
