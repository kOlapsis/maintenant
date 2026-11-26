// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestTriggers_RefuseAFilterThatMatchesNoAlert(t *testing.T) {
	withEditionPinned(t, extension.Pro)
	db := storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	channels := store.NewChannelStore(db)
	triggers := store.NewTriggerStore(db)
	h := NewAlertTriggerHandler(triggers, channels, nil)
	ctx := context.Background()

	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)

	legacyID, err := triggers.InsertTrigger(ctx, &alert.AlertTrigger{Name: "legacy", FilterSources: "container,monitor", Enabled: true, ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)

	bodies := map[string]string{
		"filter_sources": `{"name":"x","filter_sources":"container,monitor","channel_ids":["` + ch.ID + `"]}`,
		"filter_scopes":  `{"name":"x","filter_scopes":"monitor:42","channel_ids":["` + ch.ID + `"]}`,
	}
	for field, body := range bodies {
		rec := call(h.HandleCreateTrigger, http.MethodPost, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, field)
		assert.Contains(t, rec.Body.String(), "validation_failed", field)
		assert.Contains(t, rec.Body.String(), field)

	}

	rec := call(h.HandleUpdateTrigger, http.MethodPut,
		`{"name":"legacy","filter_sources":"container,monitor,probe","channel_ids":["`+ch.ID+`"]}`, "id", legacyID)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), `\"probe\"`)

	rec = call(h.HandleListTriggers, http.MethodGet, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"filter_sources":"container,monitor"`)
	rec = call(h.HandleGetTrigger, http.MethodGet, "", "id", legacyID)
	assert.Equal(t, http.StatusOK, rec.Code)

	all, err := triggers.ListTriggers(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "a refused creation leaves no trigger")
	assert.Equal(t, "legacy", all[0].Name, "a refused update leaves the trigger alone")

	rec = call(h.HandleUpdateTrigger, http.MethodPut,
		`{"name":"legacy","filter_sources":"container,swarm,kubernetes","filter_scopes":"container:0195f3c4-11aa-7f00-8c55-2b9d0e3a4c18","channel_ids":["`+ch.ID+`"]}`,
		"id", legacyID)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestTriggers_ToggleATriggerThatStoresAnUnknownSource(t *testing.T) {
	withEditionPinned(t, extension.Pro)
	db := storetest.Open(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	channels := store.NewChannelStore(db)
	triggers := store.NewTriggerStore(db)
	h := NewAlertTriggerHandler(triggers, channels, nil)
	ctx := context.Background()

	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)
	id, err := triggers.InsertTrigger(ctx, &alert.AlertTrigger{
		Name: "legacy", FilterSources: "monitor", FilterScopes: "monitor:42", Enabled: true, ChannelIDs: []string{ch.ID},
	})
	require.NoError(t, err)

	rec := call(h.HandleUpdateTrigger, http.MethodPut,
		`{"name":"legacy","filter_sources":"monitor","filter_scopes":"monitor:42","enabled":false,"channel_ids":["`+ch.ID+`"]}`, "id", id)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got, err := triggers.GetTrigger(ctx, id)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "monitor", got.FilterSources)
	assert.Equal(t, "monitor:42", got.FilterScopes)

	rec = call(h.HandleCreateTrigger, http.MethodPost,
		`{"name":"posture","filter_scopes":"infrastructure:,swarm_cluster:","channel_ids":["`+ch.ID+`"]}`)
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}
