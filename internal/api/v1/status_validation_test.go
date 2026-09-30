// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func statusAdminRouter(t *testing.T) http.Handler {
	t.Helper()
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	svc := status.NewService(status.Deps{Components: components, Logger: logger, Incidents: incidents})
	return NewRouter(HandlerDeps{
		Logger:           logger,
		StatusComponents: components,
		StatusIncidents:  incidents,
		StatusSvc:        svc,
	}).Handler()
}

func TestStatusAdmin_RefusesValuesOutsideTheModel(t *testing.T) {
	admin := statusAdminRouter(t)

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","composition_mode":"match-all","match_all_type":"endpoint"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))

	rec = serve(t, admin, http.MethodPost, "/api/v1/status/incidents", `{"title":"Down","severity":"major"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var inc status.Incident
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inc))

	cases := []struct {
		name, method, target, body, field string
	}{
		{"component override", http.MethodPut, "/api/v1/status/components/" + comp.ID, `{"status_override":"on_fire"}`, "status_override"},
		{"incident severity", http.MethodPost, "/api/v1/status/incidents", `{"title":"Down","severity":"apocalyptic"}`, "severity"},
		{"incident status", http.MethodPost, "/api/v1/status/incidents", `{"title":"Down","severity":"minor","status":"panicking"}`, "status"},
		{"incident edit severity", http.MethodPut, "/api/v1/status/incidents/" + inc.ID, `{"severity":"apocalyptic"}`, "severity"},
		{"incident update status", http.MethodPost, "/api/v1/status/incidents/" + inc.ID + "/updates", `{"status":"panicking","message":"hm"}`, "status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, admin, tc.method, tc.target, tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tc.field+" must be one of")
		})
	}

	rec = serve(t, admin, http.MethodGet, "/api/v1/status/components", "")
	var comps []status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comps))
	require.Len(t, comps, 1)
	assert.Nil(t, comps[0].StatusOverride, "a refused override must not be stored")
}

func TestStatusAdmin_PublicStreamHearsOnlyOfComponentsItShows(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	var public []string
	svc := status.NewService(status.Deps{
		Components:        components,
		Logger:            logger,
		PublicBroadcaster: func(eventType string, _ any) { public = append(public, eventType) },
	})
	admin := NewRouter(HandlerDeps{Logger: logger, StatusComponents: components, StatusSvc: svc}).Handler()
	heard := func() []string {
		got := public
		public = nil
		return got
	}

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","composition_mode":"match-all","match_all_type":"endpoint"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))
	assert.Equal(t, []string{event.StatusComponentCreated, event.StatusGlobalChanged}, heard())

	rec = serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, `{"visible":false}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{event.StatusComponentUpdated, event.StatusGlobalChanged}, heard(), "hiding a component removes it from the page")

	rec = serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, `{"display_name":"Internal API"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = serve(t, admin, http.MethodDelete, "/api/v1/status/components/"+comp.ID, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"Internal DB","composition_mode":"match-all","match_all_type":"container","visible":false}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Empty(t, heard(), "a hidden component is edited, deleted and created without the public page hearing of it")
}

type failingComponentStore struct{ status.ComponentStore }

func (failingComponentStore) CreateComponent(context.Context, *status.Component) (string, error) {
	return "", errors.New("disk full")
}

func TestStatusAdmin_FailedCreationAnswersTheLowerCaseCode(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	components := failingComponentStore{}
	admin := NewRouter(HandlerDeps{
		Logger:           logger,
		StatusComponents: components,
		StatusSvc:        status.NewService(status.Deps{Components: components, Logger: logger}),
	}).Handler()

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","composition_mode":"match-all","match_all_type":"endpoint"}`)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "internal", body.Error.Code)
	assert.NotContains(t, rec.Body.String(), "disk full")
}

func TestStatusAdmin_AcceptsEveryModelValue(t *testing.T) {
	admin := statusAdminRouter(t)

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/components",
		`{"display_name":"API","composition_mode":"match-all","match_all_type":"endpoint"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var comp status.Component
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comp))

	for _, s := range []string{"operational", "degraded", "partial_outage", "major_outage", "under_maintenance", ""} {
		rec := serve(t, admin, http.MethodPut, "/api/v1/status/components/"+comp.ID, `{"status_override":"`+s+`"}`)
		assert.Equal(t, http.StatusOK, rec.Code, "override %q: %s", s, rec.Body.String())
	}
	for _, sev := range []string{"minor", "major", "critical"} {
		rec := serve(t, admin, http.MethodPost, "/api/v1/status/incidents", `{"title":"t","severity":"`+sev+`"}`)
		require.Equal(t, http.StatusCreated, rec.Code, "severity %q: %s", sev, rec.Body.String())
		var inc status.Incident
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &inc))
		for _, st := range []string{"investigating", "identified", "monitoring", "resolved"} {
			rec := serve(t, admin, http.MethodPost, "/api/v1/status/incidents/"+inc.ID+"/updates", `{"status":"`+st+`","message":"m"}`)
			assert.Equal(t, http.StatusCreated, rec.Code, "status %q: %s", st, rec.Body.String())
		}
	}
}
