// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestStatusAdmin_RefusesComponentIDsThatNameNoComponent(t *testing.T) {
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
		`{"display_name":"API","composition_mode":"match-all","match_all_type":"endpoint"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))

	window := `{"title":"db upgrade","starts_at":"2099-01-01T02:00:00Z","ends_at":"2099-01-01T04:00:00Z","component_ids":`
	rec = serve(t, admin, http.MethodPost, "/api/v1/status/maintenance", window+`["`+comp.ID+`"]}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var mw status.MaintenanceWindow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mw))

	bad := map[string]string{
		"empty":     `[""]`,
		"unknown":   `["00000000-0000-0000-0000-00000000dead"]`,
		"repeated":  `["` + comp.ID + `","` + comp.ID + `"]`,
		"after one": `["` + comp.ID + `",""]`,
	}
	for name, ids := range bad {
		t.Run(name, func(t *testing.T) {
			for _, call := range []struct{ method, target, body string }{
				{http.MethodPost, "/api/v1/status/maintenance", window + ids + `}`},
				{http.MethodPut, "/api/v1/status/maintenance/" + mw.ID, `{"component_ids":` + ids + `}`},
				{http.MethodPost, "/api/v1/status/incidents", `{"title":"down","severity":"major","component_ids":` + ids + `}`},
			} {
				rec := serve(t, admin, call.method, call.target, call.body)
				assert.Equal(t, http.StatusBadRequest, rec.Code, "%s %s: %s", call.method, call.target, rec.Body.String())
				assert.Contains(t, rec.Body.String(), "component_ids")
			}
		})
	}

	windows, err := maintenance.ListMaintenance(t.Context(), "", 20)
	require.NoError(t, err)
	require.Len(t, windows, 1, "a refused window is not half created")
	require.Len(t, windows[0].Components, 1, "a refused update leaves the components in place")
	assert.Equal(t, comp.ID, windows[0].Components[0].ID)
	_, total, err := incidents.ListIncidents(t.Context(), status.ListIncidentsOpts{})
	require.NoError(t, err)
	assert.Zero(t, total)
}
