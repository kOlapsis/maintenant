// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

type noContainers struct{}

func (noContainers) ListContainerInfos(context.Context) ([]update.ContainerInfo, error) {
	return nil, nil
}

// hostOSFixture enrols one Debian 11 host beside the local sentinel and returns
// an update handler serving their support state on a fixed day.
func hostOSFixture(t *testing.T) (*UpdateHandler, *AgentHandler) {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "web-03", Hostname: "web-03", Label: "web-03", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))

	today := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	_, err := agentStore.UpdateAgentOS(ctx, "web-03", agent.OSIdentity{
		ID: "debian", VersionID: "11", PrettyName: "Debian GNU/Linux 11 (bullseye)", Source: "host_file",
	}, today)
	require.NoError(t, err)
	_, err = agentStore.UpdateAgentOS(ctx, uid.LocalAgent, agent.OSIdentity{
		ID: "ubuntu", VersionID: "24.04", PrettyName: "Ubuntu 24.04.1 LTS", Source: "host_file",
	}, today)
	require.NoError(t, err)

	svc, err := eol.New(eol.Deps{
		Store:  agentStore,
		Emit:   func(alert.Event) {},
		Now:    func() time.Time { return today },
		Logger: logger,
	})
	require.NoError(t, err)

	updateStore := store.NewUpdateStore(db)
	h := NewUpdateHandler(update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
		Containers: noContainers{},
		Logger:     logger,
	}), updateStore, nil)
	h.SetHostOS(svc, nil, time.Minute)

	ah := NewAgentHandler(agentStore, nil, nil, logger, "", "", time.Minute, nil)
	ah.SetEOLTables(svc)
	return h, ah
}

func TestHandleListHostOS(t *testing.T) {
	h, _ := hostOSFixture(t)

	rec := httptest.NewRecorder()
	h.HandleListHostOS(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates/hosts", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Hosts []struct {
			AgentID         string `json:"agent_id"`
			Hostname        string `json:"hostname"`
			IsLocal         bool   `json:"is_local"`
			ConnectionState string `json:"connection_state"`
			OS              struct {
				ID      string `json:"id"`
				Support struct {
					State         string `json:"state"`
					SecurityUntil string `json:"security_until"`
				} `json:"support"`
			} `json:"os"`
		} `json:"hosts"`
		EolTable struct {
			Source         string `json:"source"`
			RefreshEnabled bool   `json:"refresh_enabled"`
		} `json:"eol_table"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	require.Len(t, body.Hosts, 2)
	assert.Equal(t, "web-03", body.Hosts[0].AgentID, "the ended host comes first")
	assert.Equal(t, "ended", body.Hosts[0].OS.Support.State)
	assert.Equal(t, "2026-08-31", body.Hosts[0].OS.Support.SecurityUntil)

	local := body.Hosts[1]
	assert.Equal(t, uid.LocalAgent, local.AgentID)
	assert.True(t, local.IsLocal)
	assert.Equal(t, "connected", local.ConnectionState, "the sentinel is always connected")
	assert.Equal(t, "local", local.Hostname)
	assert.Equal(t, "ubuntu", local.OS.ID)

	assert.Equal(t, eol.SourceEmbedded, body.EolTable.Source)
	assert.False(t, body.EolTable.RefreshEnabled)
}

func TestHandleGetUpdateSummary_OSCounts(t *testing.T) {
	h, _ := hostOSFixture(t)

	rec := httptest.NewRecorder()
	h.HandleGetUpdateSummary(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates/summary", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		OSCounts map[string]int `json:"os_counts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 1, body.OSCounts["ended"])
	assert.Equal(t, 1, body.OSCounts["supported"])
	assert.Equal(t, 0, body.OSCounts["unknown"])
}

func TestHandleGetUpdateSummary_ReportsOnlyComputedCounts(t *testing.T) {
	h, _ := hostOSFixture(t)

	rec := httptest.NewRecorder()
	h.HandleGetUpdateSummary(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates/summary", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Counts map[string]int `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.ElementsMatch(t, []string{"critical", "recommended", "available", "up_to_date", "pinned"},
		slices.Collect(maps.Keys(body.Counts)))
}

func TestHandleListAgents_CarriesOSSupport(t *testing.T) {
	_, ah := hostOSFixture(t)

	rec := httptest.NewRecorder()
	ah.HandleListAgents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Agents []struct {
			AgentID string `json:"agent_id"`
			OS      struct {
				VersionID  string `json:"version_id"`
				ReportedAt string `json:"reported_at"`
				Support    struct {
					State string `json:"state"`
				} `json:"support"`
			} `json:"os"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	for _, a := range body.Agents {
		if a.AgentID != "web-03" {
			continue
		}
		assert.Equal(t, "11", a.OS.VersionID)
		assert.Equal(t, "ended", a.OS.Support.State)
		assert.NotEmpty(t, a.OS.ReportedAt)
		return
	}
	t.Fatal("agent web-03 missing from the listing")
}

type fixedContainerInfo struct {
	info update.ContainerInfo
}

func (f fixedContainerInfo) GetContainerInfo(context.Context, string, string) (update.ContainerInfo, error) {
	return f.info, nil
}

func TestHandleGetContainerUpdate_RollbackReturnsToThePreviousImage(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	updateStore := store.NewUpdateStore(db)

	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
		ScanID:         scanID,
		ContainerID:    "ctr1",
		ContainerName:  "app-web-1",
		Image:          "nginx:latest",
		CurrentTag:     "latest",
		CurrentDigest:  "sha256:old",
		Registry:       "registry-1.docker.io",
		LatestTag:      "latest",
		LatestDigest:   "sha256:new",
		UpdateType:     update.UpdateTypeDigestOnly,
		PreviousDigest: "sha256:old",
		Status:         update.StatusAvailable,
		DetectedAt:     time.Now(),
	})
	require.NoError(t, err)

	h := NewUpdateHandler(update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
		Containers: noContainers{},
		Logger:     logger,
	}), updateStore, fixedContainerInfo{info: update.ContainerInfo{
		ExternalID:         "ctr1",
		Name:               "app-web-1",
		Image:              "nginx:latest",
		OrchestrationGroup: "app",
		OrchestrationUnit:  "web",
		ComposeWorkingDir:  "/srv/app",
		RuntimeType:        "docker",
	}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/ctr1", nil)
	req.SetPathValue("container_id", "ctr1")
	rec := httptest.NewRecorder()
	h.HandleGetContainerUpdate(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		UpdateCommand   string `json:"update_command"`
		RollbackCommand string `json:"rollback_command"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEqual(t, body.UpdateCommand, body.RollbackCommand)
	assert.Contains(t, body.RollbackCommand, "docker tag nginx@sha256:old nginx:latest")
}

func TestHandleCreateExclusion_ADuplicateAnswersWithTheStoredRow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	updateStore := store.NewUpdateStore(storetest.Open(t, logger))
	h := NewUpdateHandler(nil, updateStore, nil)

	post := func() (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/updates/exclusions",
			strings.NewReader(`{"pattern":"nginx*","pattern_type":"image"}`))
		w := httptest.NewRecorder()
		h.HandleCreateExclusion(w, req)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w.Code, body
	}

	code, first := post()
	assert.Equal(t, http.StatusCreated, code)
	code, again := post()
	assert.Equal(t, http.StatusOK, code, "a duplicate creates nothing")
	assert.Equal(t, first["id"], again["id"])
	assert.Equal(t, first["created_at"], again["created_at"])

	stored, err := updateStore.ListExclusions(context.Background())
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, first["id"], stored[0].ID)
}

func TestUpdateRows_CarryTheirHost(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	agentStore := store.NewAgentStore(db)
	for _, ag := range []agent.Agent{
		{AgentID: "agent-nas", Hostname: "nas-02", Label: "NAS cave", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
		{AgentID: "agent-web", Hostname: "web-07", Status: "active", DetectedRuntime: "docker", CreatedAt: time.Now()},
	} {
		require.NoError(t, agentStore.Insert(ctx, &ag))
	}

	containerStore := store.NewContainerStore(db)
	updateStore := store.NewUpdateStore(db)
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	now := time.Now()
	for externalID, agentID := range map[string]string{
		"ctr-valkey-local": uid.LocalAgent,
		"ctr-valkey-nas":   "agent-nas",
		"ctr-valkey-web":   "agent-web",
	} {
		_, err := containerStore.InsertContainer(ctx, &container.Container{
			ExternalID: externalID, AgentID: agentID, Name: "valkey", Image: "valkey/valkey:8.0.1",
			State: container.StateRunning, AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
			RuntimeType: "docker", FirstSeenAt: now, LastStateChangeAt: now,
		})
		require.NoError(t, err)
		_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
			ScanID: scanID, ContainerID: externalID, AgentID: agentID, ContainerName: "valkey",
			Image: "valkey/valkey:8.0.1", CurrentTag: "8.0.1", Registry: "registry-1.docker.io", LatestTag: "8.1.3",
			UpdateType: update.UpdateTypeMinor, RiskScore: update.BaseRiskScore(update.UpdateTypeMinor),
			Status: update.StatusAvailable, DetectedAt: now,
		})
		require.NoError(t, err)
	}

	directory := agentStoreDirectory{store: agentStore}
	uh := NewUpdateHandler(nil, updateStore, nil)
	uh.SetAgentDirectory(directory)
	rh := NewRiskHandler(updateStore)
	rh.SetAgentDirectory(directory)

	for _, tc := range []struct {
		name, rowsKey string
		handle        http.HandlerFunc
	}{
		{"dry-run", "would_update", uh.HandleGetDryRun},
		{"risk", "containers", rh.HandleListRiskScores},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handle(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, http.StatusOK, rec.Code)

			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			var rows []map[string]any
			require.NoError(t, json.Unmarshal(body[tc.rowsKey], &rows))

			got := map[string]map[string]any{}
			for _, row := range rows {
				fields := map[string]any{}
				for _, k := range []string{"agent_id", "agent_hostname", "agent_label"} {
					if v, ok := row[k]; ok {
						fields[k] = v
					}
				}
				got[row["container_id"].(string)] = fields
			}
			assert.Equal(t, map[string]map[string]any{
				"ctr-valkey-local": {},
				"ctr-valkey-nas":   {"agent_id": "agent-nas", "agent_hostname": "nas-02", "agent_label": "NAS cave"},
				"ctr-valkey-web":   {"agent_id": "agent-web", "agent_hostname": "web-07"},
			}, got)
		})
	}
}
