// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
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
