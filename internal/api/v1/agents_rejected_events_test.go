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

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type rejectingSessions map[string]uint64

func (rejectingSessions) IsConnected(string) bool                { return true }
func (rejectingSessions) RuntimeUnreachable(string) bool         { return false }
func (rejectingSessions) Close(string, string)                   {}
func (rejectingSessions) GRPCServerCertificate() (string, bool)  { return "", false }
func (r rejectingSessions) RejectedEvents(agentID string) uint64 { return r[agentID] }

func TestAgentHandler_ReportsRejectedEvents(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	agents := store.NewAgentStore(storetest.Open(t, logger))
	require.NoError(t, agents.Insert(context.Background(), &agent.Agent{
		AgentID: "web-01", Hostname: "web-01", Label: "web-01", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))
	h := NewAgentHandler(agents, rejectingSessions{"web-01": 7}, nil, logger, "", "", time.Minute, nil)

	list := httptest.NewRecorder()
	h.HandleListAgents(list, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var listed struct {
		Agents []map[string]any `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &listed))
	require.Len(t, listed.Agents, 1)
	assert.EqualValues(t, 7, listed.Agents[0]["rejected_events"])

	get := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/web-01", nil)
	req.SetPathValue("id", "web-01")
	h.HandleGetAgent(get, req)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	var one map[string]any
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &one))
	assert.EqualValues(t, 7, one["rejected_events"])
}

type unreachableRuntimeSessions map[string]bool

func (unreachableRuntimeSessions) IsConnected(string) bool                  { return true }
func (u unreachableRuntimeSessions) RuntimeUnreachable(agentID string) bool { return u[agentID] }
func (unreachableRuntimeSessions) Close(string, string)                     {}
func (unreachableRuntimeSessions) GRPCServerCertificate() (string, bool)    { return "", false }

func TestAgentHandler_ReportsAnUnreachableRuntime(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	agents := store.NewAgentStore(storetest.Open(t, logger))
	for _, id := range []string{"web-01", "web-02"} {
		require.NoError(t, agents.Insert(context.Background(), &agent.Agent{
			AgentID: id, Hostname: id, Label: id, Status: "active",
			DetectedRuntime: "docker", CreatedAt: time.Now(),
		}))
	}
	h := NewAgentHandler(agents, unreachableRuntimeSessions{"web-01": true}, nil, logger, "", "", time.Minute, nil)

	rec := httptest.NewRecorder()
	h.HandleListAgents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var listed struct {
		Agents []map[string]any `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Agents, 2)
	for _, a := range listed.Agents {
		assert.Equal(t, a["agent_id"] == "web-01", a["runtime_unreachable"], a["agent_id"])
	}

	noSessions := NewAgentHandler(agents, nil, nil, logger, "", "", time.Minute, nil)
	rec = httptest.NewRecorder()
	noSessions.HandleListAgents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	for _, a := range listed.Agents {
		assert.Equal(t, false, a["runtime_unreachable"])
	}
}
