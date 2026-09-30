// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package statuspage

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type maintenanceBench struct {
	scheduler   *MaintenanceScheduler
	components  *store.StatusComponentStoreImpl
	incidents   *store.IncidentStoreImpl
	maintenance *store.MaintenanceStoreImpl
}

func newMaintenanceBench(t *testing.T) *maintenanceBench {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	b := &maintenanceBench{
		components:  store.NewStatusComponentStore(db),
		incidents:   store.NewIncidentStore(db),
		maintenance: store.NewMaintenanceStore(db),
	}
	svc := status.NewService(status.Deps{
		Components: b.components, Logger: logger, Incidents: b.incidents, Maintenance: b.maintenance,
	})
	b.scheduler = NewMaintenanceScheduler(b.maintenance, b.components, b.incidents, svc, logger)
	return b
}

func (b *maintenanceBench) component(t *testing.T, override *string) string {
	t.Helper()
	c := &status.Component{
		CompositionMode: status.CompositionMatchAll, MatchAllType: "container",
		DisplayName: "API", Visible: true, StatusOverride: override,
	}
	id, err := b.components.CreateComponent(context.Background(), c)
	require.NoError(t, err)
	return id
}

func (b *maintenanceBench) window(t *testing.T, componentIDs ...string) *status.MaintenanceWindow {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	id, err := b.maintenance.CreateMaintenance(ctx, &status.MaintenanceWindow{
		Title: "db upgrade", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	}, componentIDs)
	require.NoError(t, err)
	return b.reload(t, id)
}

func (b *maintenanceBench) reload(t *testing.T, windowID string) *status.MaintenanceWindow {
	t.Helper()
	mw, err := b.maintenance.GetMaintenance(context.Background(), windowID)
	require.NoError(t, err)
	require.NotNil(t, mw)
	return mw
}

func (b *maintenanceBench) override(t *testing.T, componentID string) *string {
	t.Helper()
	c, err := b.components.GetComponent(context.Background(), componentID)
	require.NoError(t, err)
	return c.StatusOverride
}

func ptr(s string) *string { return &s }

func TestMaintenance_EndRestoresTheManualOverride(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, ptr(status.StatusDegraded))
	mw := b.window(t, comp)

	b.scheduler.activateWindow(ctx, mw)
	assert.Equal(t, ptr(status.StatusUnderMaint), b.override(t, comp))

	b.scheduler.deactivateWindow(ctx, b.reload(t, mw.ID))
	assert.Equal(t, ptr(status.StatusDegraded), b.override(t, comp), "the manual override outlives the window")
}

func TestMaintenance_EndWithoutManualOverrideClearsIt(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, nil)
	mw := b.window(t, comp)

	b.scheduler.activateWindow(ctx, mw)
	b.scheduler.deactivateWindow(ctx, b.reload(t, mw.ID))
	assert.Nil(t, b.override(t, comp))
}

func TestMaintenance_OverlappingWindowsHoldTheComponentUntilTheLastEnds(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, ptr(status.StatusDegraded))
	first := b.window(t, comp)
	second := b.window(t, comp)

	b.scheduler.activateWindow(ctx, first)
	b.scheduler.activateWindow(ctx, second)

	b.scheduler.deactivateWindow(ctx, b.reload(t, first.ID))
	assert.Equal(t, ptr(status.StatusUnderMaint), b.override(t, comp), "the second window still runs")

	b.scheduler.deactivateWindow(ctx, b.reload(t, second.ID))
	assert.Equal(t, ptr(status.StatusDegraded), b.override(t, comp))
}

func TestMaintenance_OverrideChangedDuringTheWindowStays(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, ptr(status.StatusDegraded))
	mw := b.window(t, comp)

	b.scheduler.activateWindow(ctx, mw)
	c, err := b.components.GetComponent(ctx, comp)
	require.NoError(t, err)
	c.StatusOverride = ptr(status.StatusMajorOutage)
	require.NoError(t, b.components.UpdateComponent(ctx, c))

	b.scheduler.deactivateWindow(ctx, b.reload(t, mw.ID))
	assert.Equal(t, ptr(status.StatusMajorOutage), b.override(t, comp))
}

func TestMaintenance_DeletingARunningWindowEndsIt(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, ptr(status.StatusDegraded))
	mw := b.window(t, comp)
	b.scheduler.activateWindow(ctx, mw)
	running := b.reload(t, mw.ID)
	require.True(t, running.Active)
	require.NotNil(t, running.IncidentID)

	require.NoError(t, b.scheduler.DeleteWindow(ctx, mw.ID))

	gone, err := b.maintenance.GetMaintenance(ctx, mw.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, ptr(status.StatusDegraded), b.override(t, comp))
	inc, err := b.incidents.GetIncident(ctx, *running.IncidentID)
	require.NoError(t, err)
	assert.Equal(t, status.IncidentResolved, inc.Status)
}

type eventRecorder struct {
	mu     sync.Mutex
	events map[string]map[string]any
}

func (r *eventRecorder) broadcast(eventType string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.events == nil {
		r.events = map[string]map[string]any{}
	}
	r.events[eventType], _ = data.(map[string]any)
}

func (r *eventRecorder) components(eventType string) any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[eventType]["components"]
}

func TestMaintenance_PublicSurfacesNameOnlyVisibleComponents(t *testing.T) {
	pinEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	maintenance := store.NewMaintenanceStore(db)
	public, admin := &eventRecorder{}, &eventRecorder{}
	svc := status.NewService(status.Deps{
		Components: components, Logger: logger, Incidents: incidents, Maintenance: maintenance,
		PublicBroadcaster: public.broadcast, AdminBroadcaster: admin.broadcast,
	})
	svc.SetSubscriberService(status.NewSubscriberService(&confirmedSubscribers{}, &fakeMailer{}, "https://status.example.com", logger))
	n := &recordingNotifier{calls: make(chan notifyCall, 8)}
	svc.SetSubscriberNotifier(n)
	scheduler := NewMaintenanceScheduler(maintenance, components, incidents, svc, logger)

	ctx := context.Background()
	var ids []string
	for _, c := range []*status.Component{
		{CompositionMode: status.CompositionMatchAll, MatchAllType: "endpoint", DisplayName: "API", Visible: true},
		{CompositionMode: status.CompositionMatchAll, MatchAllType: "container", DisplayName: "Internal DB"},
	} {
		id, err := components.CreateComponent(ctx, c)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	now := time.Now().UTC()
	windowID, err := maintenance.CreateMaintenance(ctx, &status.MaintenanceWindow{
		Title: "db upgrade", StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	}, ids)
	require.NoError(t, err)
	mw, err := maintenance.GetMaintenance(ctx, windowID)
	require.NoError(t, err)

	scheduler.activateWindow(ctx, mw)
	started := n.next(t)
	assert.Contains(t, started.message, "Affected components: API\n")
	assert.NotContains(t, started.message, "Internal DB")

	mw, err = maintenance.GetMaintenance(ctx, windowID)
	require.NoError(t, err)
	scheduler.deactivateWindow(ctx, mw)

	for _, evt := range []string{event.StatusMaintenanceStart, event.StatusMaintenanceEnd} {
		assert.Equal(t, []string{"API"}, public.components(evt), "%s on the public page", evt)
		assert.ElementsMatch(t, []string{"API", "Internal DB"}, admin.components(evt), "%s on the dashboard", evt)
	}
}

func TestMaintenance_DeletingAScheduledWindowLeavesTheComponent(t *testing.T) {
	b := newMaintenanceBench(t)
	ctx := context.Background()
	comp := b.component(t, ptr(status.StatusDegraded))
	mw := b.window(t, comp)

	require.NoError(t, b.scheduler.DeleteWindow(ctx, mw.ID))

	gone, err := b.maintenance.GetMaintenance(ctx, mw.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, ptr(status.StatusDegraded), b.override(t, comp))
	_, total, err := b.incidents.ListIncidents(ctx, status.ListIncidentsOpts{})
	require.NoError(t, err)
	assert.Zero(t, total, "a window that never ran has no incident to resolve")
}
