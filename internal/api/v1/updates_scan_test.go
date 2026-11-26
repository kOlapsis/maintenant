// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/update"
)

type heldContainers struct{ release chan struct{} }

func (h heldContainers) ListContainerInfos(context.Context) ([]update.ContainerInfo, error) {
	<-h.release
	return nil, nil
}

func TestHandleTriggerScan_AnswersRunningOnlyForTheScanItStarted(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	updateStore := store.NewUpdateStore(storetest.Open(t, logger))
	containers := heldContainers{release: make(chan struct{})}
	svc := update.NewService(update.Deps{
		Store:      updateStore,
		Scanner:    update.NewScanner(update.NewRegistryClient(), updateStore, logger),
		Containers: containers,
		Logger:     logger,
	})
	h := NewUpdateHandler(svc, updateStore, nil)
	post := func() int {
		w := httptest.NewRecorder()
		h.HandleTriggerScan(w, httptest.NewRequest(http.MethodPost, "/api/v1/updates/scan", nil))
		return w.Code
	}

	require.Equal(t, http.StatusAccepted, post())
	assert.True(t, svc.IsScanning(), "202 means a scan is running")
	assert.Equal(t, http.StatusConflict, post())

	close(containers.release)
	svc.Wait()
	assert.Equal(t, http.StatusAccepted, post())
	svc.Wait()
}
