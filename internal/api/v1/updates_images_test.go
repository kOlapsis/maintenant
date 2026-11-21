// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

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
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

type postgresTable struct{}

func (postgresTable) Current() eol.Table {
	ended, _ := eol.ParseDate("2020-01-01")
	supported, _ := eol.ParseDate("2099-01-01")
	return eol.Table{Source: "test", Products: map[string][]eol.Cycle{"postgresql": {
		{Name: "9", SecurityUntil: ended},
		{Name: "16", SecurityUntil: supported},
	}}}
}

func imageSupportHandler(t *testing.T, containers []update.ContainerInfo) http.Handler {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	agentStore := store.NewAgentStore(db)
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "agent-nas", Hostname: "nas-02", Label: "NAS cave", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))

	updateStore := store.NewUpdateStore(db)
	svc := update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
		Containers: &scanLister{infos: containers},
		Logger:     logger,
		EOL:        postgresTable{},
	})
	containerSvc := container.NewService(container.Deps{Store: store.NewContainerStore(db), Logger: logger})
	return NewRouter(HandlerDeps{
		Logger:           logger,
		Containers:       containerSvc,
		UpdateSvc:        svc,
		UpdateStore:      updateStore,
		ContainerAdapter: update.NewContainerServiceAdapter(containerSvc, logger),
		AgentStore:       agentStore,
	}).Handler()
}

func getImages(t *testing.T, h http.Handler) (map[string]json.RawMessage, []map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates/images", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	var images []map[string]any
	require.NoError(t, json.Unmarshal(raw["images"], &images))
	return raw, images
}

func TestHandleListImageSupport_ItemsAndAgentFields(t *testing.T) {
	h := imageSupportHandler(t, []update.ContainerInfo{
		{ExternalID: "ctr-local", Name: "db", Image: "postgres:9.1.2"},
		{ExternalID: "ctr-nas", AgentID: "agent-nas", Name: "db", Image: "postgres:16.3", Labels: map[string]string{"maintenant.update.enabled": "false"}},
		{ExternalID: "ctr-other", Name: "cache", Image: "acme/postgres:16.3"},
		{ExternalID: "ctr-built", Name: "built", Image: "postgres:16.3", LocallyBuilt: true},
	})

	_, images := getImages(t, h)
	require.Len(t, images, 2)

	local := images[0]
	assert.ElementsMatch(t, []string{"container_id", "container_name", "image", "support"}, imageItemKeys(local), "a local item carries no agent field")
	assert.Equal(t, "ctr-local", local["container_id"])
	assert.Equal(t, "db", local["container_name"])
	assert.Equal(t, "postgres:9.1.2", local["image"])
	support, ok := local["support"].(map[string]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{
		"state", "product", "cycle", "active_until", "security_until", "extended_until", "days_remaining", "table_source",
	}, imageItemKeys(support))
	assert.Equal(t, "ended", support["state"])
	assert.Equal(t, "postgresql", support["product"])
	assert.Equal(t, "9", support["cycle"])
	assert.Equal(t, "2020-01-01", support["security_until"])
	assert.Equal(t, "test", support["table_source"])

	remote := images[1]
	assert.ElementsMatch(t, []string{
		"container_id", "container_name", "image", "support", "agent_id", "agent_hostname", "agent_label",
	}, imageItemKeys(remote))
	assert.Equal(t, "ctr-nas", remote["container_id"])
	assert.Equal(t, "agent-nas", remote["agent_id"])
	assert.Equal(t, "nas-02", remote["agent_hostname"])
	assert.Equal(t, "NAS cave", remote["agent_label"])
	assert.Equal(t, "supported", remote["support"].(map[string]any)["state"])
	assert.NotEqual(t, uid.LocalAgent, remote["agent_id"])
}

func TestHandleListImageSupport_EmptyIsAnArray(t *testing.T) {
	raw, images := getImages(t, imageSupportHandler(t, nil))
	assert.JSONEq(t, `[]`, string(raw["images"]))
	assert.Empty(t, images)
}

func imageItemKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
