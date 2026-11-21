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

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestUpdates_MajorTagAndSupportOfTheRunningImage(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	containerStore := store.NewContainerStore(db)
	updateStore := store.NewUpdateStore(db)
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)

	now := time.Now()
	for _, u := range []update.ImageUpdate{
		{ContainerID: "ctr-pg", ContainerName: "pg", Image: "postgres:13.14", CurrentTag: "13.14", LatestTag: "13.15", MajorTag: "19.0", UpdateType: update.UpdateTypePatch},
		{ContainerID: "ctr-app", ContainerName: "app", Image: "acme/postgres:16.1", CurrentTag: "16.1", LatestTag: "16.4", UpdateType: update.UpdateTypeMinor},
	} {
		_, err := containerStore.InsertContainer(ctx, &container.Container{
			ExternalID: u.ContainerID, AgentID: uid.LocalAgent, Name: u.ContainerName, Image: u.Image,
			State: container.StateRunning, AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
			RuntimeType: "docker", FirstSeenAt: now, LastStateChangeAt: now,
		})
		require.NoError(t, err)
		u.ScanID, u.Registry, u.Status, u.DetectedAt = scanID, "registry-1.docker.io", update.StatusAvailable, now
		_, err = updateStore.InsertImageUpdate(ctx, &u)
		require.NoError(t, err)
	}

	svc, err := eol.New(eol.Deps{
		Store:  store.NewAgentStore(db),
		Emit:   func(alert.Event) {},
		Logger: logger,
	})
	require.NoError(t, err)
	h := NewUpdateHandler(update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
		Containers: noContainers{},
		Logger:     logger,
	}), updateStore, fixedContainerInfo{})
	h.SetHostOS(svc, nil, time.Minute)

	rec := httptest.NewRecorder()
	h.HandleListUpdates(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Updates []map[string]any `json:"updates"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	byID := map[string]map[string]any{}
	for _, u := range body.Updates {
		byID[u["container_id"].(string)] = u
	}
	require.Len(t, byID, 2)

	pg := byID["ctr-pg"]
	assert.Equal(t, "19.0", pg["major_tag"])
	support, ok := pg["support"].(map[string]any)
	require.True(t, ok, "an official image of a tracked product carries its support: %v", pg)
	assert.Equal(t, "ended", support["state"])
	assert.Equal(t, "postgresql", support["product"])
	assert.Equal(t, "13", support["cycle"])
	assert.Equal(t, "2025-11-13", support["security_until"])
	assert.Equal(t, eol.SourceEmbedded, support["table_source"])

	app := byID["ctr-app"]
	assert.NotContains(t, app, "major_tag")
	assert.NotContains(t, app, "support", "an image that is not official is untracked")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/container/ctr-pg", nil)
	req.SetPathValue("container_id", "ctr-pg")
	rec = httptest.NewRecorder()
	h.HandleGetContainerUpdate(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "19.0", detail["major_tag"])
	assert.Contains(t, detail, "support")

	h.SetHostOS(nil, nil, time.Minute)
	rec = httptest.NewRecorder()
	h.HandleListUpdates(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates", nil))
	var untracked struct {
		Updates []map[string]any `json:"updates"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &untracked))
	require.Len(t, untracked.Updates, 2)
	for _, u := range untracked.Updates {
		assert.NotContains(t, u, "support", "no cycle matches without a table")
	}
}
