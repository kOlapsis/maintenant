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

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
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
