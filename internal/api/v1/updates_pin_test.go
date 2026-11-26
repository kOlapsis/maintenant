// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestHandleCreateExclusion_RefusesAPatternThatCanNeverMatch(t *testing.T) {
	updateStore := store.NewUpdateStore(storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil))))
	h := NewUpdateHandler(nil, updateStore, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/updates/exclusions",
		strings.NewReader(`{"pattern":"nginx[","pattern_type":"image"}`))
	w := httptest.NewRecorder()
	h.HandleCreateExclusion(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_PATTERN")
	stored, err := updateStore.ListExclusions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func TestHandlePinVersion_RecordsTheNameTheRecreatedContainerKeeps(t *testing.T) {
	ctx := context.Background()
	updateStore := store.NewUpdateStore(storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil))))
	scanID, err := updateStore.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: time.Now(), Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	_, err = updateStore.InsertImageUpdate(ctx, &update.ImageUpdate{
		ScanID: scanID, ContainerID: "task1", ContainerName: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.25.3",
		CurrentTag: "1.25.3", Registry: "registry-1.docker.io", LatestTag: "1.27.0", UpdateType: update.UpdateTypeMinor,
		Status: update.StatusAvailable, DetectedAt: time.Now(),
	})
	require.NoError(t, err)
	h := NewUpdateHandler(nil, updateStore, fixedContainerInfo{info: update.ContainerInfo{
		ExternalID: "task1", Name: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.25.3", SwarmService: "shop_web",
	}})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/updates/pin/task1", strings.NewReader(`{"reason":"frozen"}`))
	req.SetPathValue("container_id", "task1")
	w := httptest.NewRecorder()
	h.HandlePinVersion(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	pin, err := updateStore.AdoptVersionPin(ctx, "", "task2", "shop_web")
	require.NoError(t, err)
	require.NotNil(t, pin, "the task that replaces a pinned one is pinned")
	assert.Equal(t, "frozen", pin.Reason)
}
