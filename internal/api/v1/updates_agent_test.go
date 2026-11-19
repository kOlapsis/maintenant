// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

const homonymWorkload = "shop/Deployment/api"

type homonymFixture struct {
	handler http.Handler
	updates *store.UpdateStore
}

func newHomonymFixture(t *testing.T, agents ...string) *homonymFixture {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	containerStore := store.NewContainerStore(db)
	updateStore := store.NewUpdateStore(db)
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	now := time.Now()
	for _, agentID := range agents {
		if agentID != uid.LocalAgent {
			require.NoError(t, agentStore.Insert(ctx, &agent.Agent{AgentID: agentID, Hostname: agentID, Status: "active",
				DetectedRuntime: "kubernetes", CreatedAt: now}))
		}
		_, err := containerStore.InsertContainer(ctx, &container.Container{
			ExternalID: homonymWorkload, AgentID: agentID, Name: "api", Image: "shop/api:1.0.0",
			State: container.StateRunning, AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
			RuntimeType: "kubernetes", FirstSeenAt: now, LastStateChangeAt: now,
		})
		require.NoError(t, err)
		_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
			ScanID: scanID, ContainerID: homonymWorkload, AgentID: agentID, ContainerName: "api",
			Image: "shop/api:1.0.0", CurrentTag: "1.0.0", Registry: "ghcr.io", LatestTag: "1.1.0",
			UpdateType: update.UpdateTypeMinor, RiskScore: update.BaseRiskScore(update.UpdateTypeMinor),
			Status: update.StatusAvailable, DetectedAt: now,
		})
		require.NoError(t, err)
		require.NoError(t, updateStore.UpsertContainerCVE(ctx, &update.ContainerCVE{AgentID: agentID,
			ContainerID: homonymWorkload, CVEID: "CVE-" + agentID, Severity: update.CVESeverityHigh, FirstDetectedAt: now}))
	}

	containerSvc := container.NewService(container.Deps{Store: containerStore, Logger: logger})
	r := NewRouter(HandlerDeps{
		Logger:     logger,
		Containers: containerSvc,
		UpdateSvc: update.NewService(update.Deps{
			Store:      updateStore,
			Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
			Containers: noContainers{},
			Logger:     logger,
		}),
		UpdateStore:      updateStore,
		ContainerAdapter: update.NewContainerServiceAdapter(containerSvc, logger),
		AgentStore:       agentStore,
	})
	return &homonymFixture{handler: r.Handler(), updates: updateStore}
}

func (f *homonymFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func (f *homonymFixture) pinned(t *testing.T, agentID string) bool {
	t.Helper()
	rec := f.do(t, http.MethodGet, "/api/v1/updates/container/"+homonymWorkload+"?agent_id="+agentID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	if agentID == uid.LocalAgent {
		assert.NotContains(t, body, "agent_id")
	} else {
		assert.Equal(t, agentID, body["agent_id"])
	}
	pin, err := f.updates.GetVersionPin(context.Background(), agentID, homonymWorkload)
	require.NoError(t, err)
	assert.Equal(t, body["pinned"], pin != nil, "the detail and the store agree for %s", agentID)
	u, err := f.updates.GetAgentImageUpdate(context.Background(), agentID, homonymWorkload)
	require.NoError(t, err)
	assert.Equal(t, body["pinned"] == true, u.Status == update.StatusPinned, "the entry status follows the pin of %s", agentID)
	return body["pinned"] == true
}

// Pinning then unpinning the workload of a remote agent leaves its homonym on another agent as it was.
func TestUpdatesAPI_PinIsPerAgent(t *testing.T) {
	f := newHomonymFixture(t, "agent-staging", "agent-prod")

	rec := f.do(t, http.MethodPost, "/api/v1/updates/pin/"+homonymWorkload+"?agent_id=agent-prod", `{"reason":"prod freeze"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.pinned(t, "agent-prod"))
	assert.False(t, f.pinned(t, "agent-staging"), "pinning prod does not pin staging")

	rec = f.do(t, http.MethodPost, "/api/v1/updates/pin/"+homonymWorkload+"?agent_id=agent-staging", `{"reason":"staging freeze"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = f.do(t, http.MethodDelete, "/api/v1/updates/pin/"+homonymWorkload+"?agent_id=agent-prod", "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.False(t, f.pinned(t, "agent-prod"))
	assert.True(t, f.pinned(t, "agent-staging"), "unpinning prod keeps the staging pin")

	pin, err := f.updates.GetVersionPin(context.Background(), "agent-staging", homonymWorkload)
	require.NoError(t, err)
	assert.Equal(t, "staging freeze", pin.Reason)
}

// Without agent_id, a container two remote agents run answers 400 on every route, and nothing is pinned.
func TestUpdatesAPI_RemoteHomonymsNeedTheAgent(t *testing.T) {
	withEdition(t, extension.Pro)
	f := newHomonymFixture(t, "agent-staging", "agent-prod")

	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/updates/container/" + homonymWorkload},
		{http.MethodPost, "/api/v1/updates/pin/" + homonymWorkload},
		{http.MethodDelete, "/api/v1/updates/pin/" + homonymWorkload},
		{http.MethodGet, "/api/v1/cve/" + homonymWorkload},
		{http.MethodGet, "/api/v1/cve?container_id=" + homonymWorkload},
		{http.MethodGet, "/api/v1/risk/" + homonymWorkload},
		{http.MethodGet, "/api/v1/risk/" + homonymWorkload + "?period=7d"},
	} {
		rec := f.do(t, call.method, call.path, `{}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "%s %s: %s", call.method, call.path, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "AGENT_REQUIRED", "%s %s", call.method, call.path)
	}
	assert.False(t, f.pinned(t, "agent-prod"))
	assert.False(t, f.pinned(t, "agent-staging"))
}

// Without agent_id, the local runtime is the one acted on when it runs the container, even beside a remote homonym.
func TestUpdatesAPI_LocalHomonymIsTheDefault(t *testing.T) {
	f := newHomonymFixture(t, uid.LocalAgent, "agent-prod")

	rec := f.do(t, http.MethodPost, "/api/v1/updates/pin/"+homonymWorkload, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.pinned(t, uid.LocalAgent))
	assert.False(t, f.pinned(t, "agent-prod"))

	rec = f.do(t, http.MethodDelete, "/api/v1/updates/pin/"+homonymWorkload, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.False(t, f.pinned(t, uid.LocalAgent))
}

// Without agent_id, a container only one remote agent runs is that agent's.
func TestUpdatesAPI_OnlyRemoteIsTheDefault(t *testing.T) {
	f := newHomonymFixture(t, "agent-prod")

	rec := f.do(t, http.MethodPost, "/api/v1/updates/pin/"+homonymWorkload, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.pinned(t, "agent-prod"))

	rec = f.do(t, http.MethodDelete, "/api/v1/updates/pin/"+homonymWorkload, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.False(t, f.pinned(t, "agent-prod"))
}

// Each homonym reads its own CVEs, in the container view and in the filtered list.
func TestCVEAPI_ReadsTheCVEsOfTheNamedAgent(t *testing.T) {
	withEdition(t, extension.Pro)
	f := newHomonymFixture(t, "agent-staging", "agent-prod")

	for _, agentID := range []string{"agent-staging", "agent-prod"} {
		rec := f.do(t, http.MethodGet, "/api/v1/cve/"+homonymWorkload+"?agent_id="+agentID, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var one struct {
			CVEs []struct {
				CVEID string `json:"cve_id"`
			} `json:"cves"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &one))
		require.Len(t, one.CVEs, 1)
		assert.Equal(t, "CVE-"+agentID, one.CVEs[0].CVEID)

		rec = f.do(t, http.MethodGet, "/api/v1/cve?container_id="+homonymWorkload+"&agent_id="+agentID, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var list struct {
			CVEs []struct {
				CVEID    string `json:"cve_id"`
				Affected []struct {
					ContainerID string `json:"container_id"`
					AgentID     string `json:"agent_id"`
				} `json:"affected_containers"`
			} `json:"cves"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list.CVEs, 1)
		assert.Equal(t, "CVE-"+agentID, list.CVEs[0].CVEID)
		require.Len(t, list.CVEs[0].Affected, 1)
		assert.Equal(t, agentID, list.CVEs[0].Affected[0].AgentID, "a remote container names its agent")
	}
}
