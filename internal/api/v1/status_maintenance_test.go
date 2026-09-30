// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/statuspage"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestDeleteMaintenance_RunningWindowEndsLikeItsScheduledEnd(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	maintenance := store.NewMaintenanceStore(db)
	svc := status.NewService(status.Deps{Components: components, Logger: logger, Incidents: incidents, Maintenance: maintenance})
	scheduler := statuspage.NewMaintenanceScheduler(maintenance, components, incidents, svc, logger)
	admin := NewRouter(HandlerDeps{
		Logger:            logger,
		StatusComponents:  components,
		StatusIncidents:   incidents,
		StatusMaintenance: maintenance,
		StatusMaintRunner: scheduler,
		StatusSvc:         svc,
	}).Handler()

	ctx := context.Background()
	degraded := status.StatusDegraded
	comp := &status.Component{CompositionMode: status.CompositionMatchAll, MatchAllType: "endpoint",
		DisplayName: "API", Visible: true, StatusOverride: &degraded}
	_, err := components.CreateComponent(ctx, comp)
	require.NoError(t, err)
	now := time.Now().UTC()
	windowID, err := maintenance.CreateMaintenance(ctx, &status.MaintenanceWindow{
		Title: "db upgrade", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	}, []string{comp.ID})
	require.NoError(t, err)

	runCtx, stop := context.WithCancel(ctx)
	go scheduler.Start(runCtx)
	var running *status.MaintenanceWindow
	require.Eventually(t, func() bool {
		running, err = maintenance.GetMaintenance(ctx, windowID)
		return err == nil && running != nil && running.Active
	}, 5*time.Second, 10*time.Millisecond)
	stop()
	require.NotNil(t, running.IncidentID)

	rec := serve(t, admin, http.MethodDelete, "/api/v1/status/maintenance/"+windowID, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	got, err := components.GetComponent(ctx, comp.ID)
	require.NoError(t, err)
	require.NotNil(t, got.StatusOverride)
	assert.Equal(t, status.StatusDegraded, *got.StatusOverride, "the component leaves maintenance with its own override")
	inc, err := incidents.GetIncident(ctx, *running.IncidentID)
	require.NoError(t, err)
	assert.Equal(t, status.IncidentResolved, inc.Status, "the maintenance incident is closed")
}

func TestUpdateMaintenance_RefusesAnEndBeforeTheStart(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	maintenance := store.NewMaintenanceStore(db)
	admin := NewRouter(HandlerDeps{
		Logger:            logger,
		StatusComponents:  components,
		StatusMaintenance: maintenance,
		StatusSvc:         status.NewService(status.Deps{Components: components, Logger: logger, Maintenance: maintenance}),
	}).Handler()

	rec := serve(t, admin, http.MethodPost, "/api/v1/status/maintenance",
		`{"title":"db upgrade","starts_at":"2099-01-01T02:00:00Z","ends_at":"2099-01-01T04:00:00Z"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var mw status.MaintenanceWindow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mw))

	for name, body := range map[string]string{
		"end moved before the start": `{"ends_at":"2099-01-01T01:00:00Z"}`,
		"start moved after the end":  `{"starts_at":"2099-01-01T05:00:00Z"}`,
		"both swapped":               `{"starts_at":"2099-01-01T04:00:00Z","ends_at":"2099-01-01T02:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(t, admin, http.MethodPut, "/api/v1/status/maintenance/"+mw.ID, body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"validation"`)
		})
	}

	got, err := maintenance.GetMaintenance(t.Context(), mw.ID)
	require.NoError(t, err)
	assert.True(t, got.StartsAt.Equal(mw.StartsAt) && got.EndsAt.Equal(mw.EndsAt), "a refused update leaves the window as it was")

	rec = serve(t, admin, http.MethodPut, "/api/v1/status/maintenance/"+mw.ID, `{"ends_at":"2099-01-01T02:00:00Z"}`)
	require.Equal(t, http.StatusOK, rec.Code, "a window may end when it starts, as on creation: %s", rec.Body.String())
}
