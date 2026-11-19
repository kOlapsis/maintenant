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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestAlertTriggers_RefuseARepeatedChannelWithoutWritingAnything(t *testing.T) {
	withEdition(t, extension.Pro)
	db := storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	triggers := store.NewTriggerStore(db)
	channels := store.NewChannelStore(db)
	h := NewAlertTriggerHandler(triggers, channels, nil)

	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)
	other := &alert.NotificationChannel{Name: "dev", Type: "webhook", URL: "https://example.com/dev", Enabled: true}
	_, err = channels.InsertChannel(ctx, other)
	require.NoError(t, err)

	create := func(name string, ids string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/alert-triggers",
			strings.NewReader(`{"name":"`+name+`","channel_ids":`+ids+`}`))
		rec := httptest.NewRecorder()
		h.HandleCreateTrigger(rec, req)
		return rec
	}

	rec := create("dup", `["`+ch.ID+`","`+ch.ID+`"]`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "channel_ids")
	all, err := triggers.ListTriggers(ctx)
	require.NoError(t, err)
	assert.Empty(t, all, "a refused creation leaves no trigger")

	rec = create("kept", `["`+ch.ID+`"]`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created alert.AlertTrigger
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/alert-triggers/"+created.ID,
		strings.NewReader(`{"name":"renamed","channel_ids":["`+other.ID+`","`+other.ID+`"]}`))
	req.SetPathValue("id", created.ID)
	rec = httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	got, err := triggers.GetTrigger(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "kept", got.Name)
	assert.Equal(t, []string{ch.ID}, got.ChannelIDs)
}

func TestStatusAdmin_RefusesARepeatedMonitorOnCreation(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	maintenance := store.NewMaintenanceStore(db)
	admin := NewRouter(HandlerDeps{
		Logger:            logger,
		StatusComponents:  components,
		StatusIncidents:   incidents,
		StatusMaintenance: maintenance,
		StatusSvc:         status.NewService(status.Deps{Components: components, Logger: logger, Incidents: incidents, Maintenance: maintenance}),
	}).Handler()

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","monitors":[{"type":"endpoint","id":"e1"},{"type":"endpoint","id":"e1"}]}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "monitors[1]")

	all, err := components.ListComponents(t.Context())
	require.NoError(t, err)
	assert.Empty(t, all)

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","monitors":[{"type":"endpoint","id":"e1"},{"type":"container","id":"e1"}]}`)
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestCreateComponent_ConcurrentCallsFillTheCapOnce(t *testing.T) {
	withEdition(t, extension.Community)
	create := newCreator(t, extension.ResourceStatusComponents)
	for i := 0; i < 2; i++ {
		require.Equal(t, http.StatusCreated, create(i).Code)
	}

	codes := make(chan int, 12)
	var wg sync.WaitGroup
	for i := 2; i < 14; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- create(i).Code
		}()
	}
	wg.Wait()
	close(codes)

	created := 0
	for code := range codes {
		if code == http.StatusCreated {
			created++
		}
	}
	assert.Equal(t, 1, created, "one place is left under the cap of three")
}
