// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

func createWindow(t *testing.T, s *MaintenanceStoreImpl, title string, startsAt, endsAt time.Time) string {
	t.Helper()
	id, err := s.CreateMaintenance(context.Background(),
		&status.MaintenanceWindow{Title: title, StartsAt: startsAt, EndsAt: endsAt}, nil)
	require.NoError(t, err)
	return id
}

func windowTitles(ws []status.MaintenanceWindow) []string {
	titles := make([]string, len(ws))
	for i, w := range ws {
		titles[i] = w.Title
	}
	return titles
}

func TestListMaintenance_UpcomingSoonestFirst(t *testing.T) {
	db := openTestDB(t)
	s := NewMaintenanceStore(db)
	now := time.Now().UTC().Truncate(time.Second)

	for _, days := range []int{7, 2, 6, 1, 5, 3, 4} {
		start := now.Add(time.Duration(days) * 24 * time.Hour)
		createWindow(t, s, fmt.Sprintf("in %d days", days), start, start.Add(time.Hour))
	}

	got, err := s.ListMaintenance(context.Background(), "upcoming", 5)
	require.NoError(t, err)
	assert.Equal(t, []string{"in 1 days", "in 2 days", "in 3 days", "in 4 days", "in 5 days"}, windowTitles(got),
		"the public page shows the next five windows, not the five farthest")
}

func TestListMaintenance_AdminListsActiveThenUpcomingThenCompleted(t *testing.T) {
	db := openTestDB(t)
	s := NewMaintenanceStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	day := 24 * time.Hour

	createWindow(t, s, "done long ago", now.Add(-10*day), now.Add(-10*day+time.Hour))
	createWindow(t, s, "next month", now.Add(30*day), now.Add(30*day+time.Hour))
	createWindow(t, s, "done yesterday", now.Add(-day), now.Add(-day+time.Hour))
	running := createWindow(t, s, "running", now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, s.SetActive(ctx, running, true, nil))
	createWindow(t, s, "tomorrow", now.Add(day), now.Add(day+time.Hour))

	got, err := s.ListMaintenance(ctx, "", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"running", "tomorrow", "next month", "done yesterday", "done long ago"}, windowTitles(got))

	got, err = s.ListMaintenance(ctx, "completed", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"done yesterday", "done long ago"}, windowTitles(got))
}

func TestMaintenance_CoveredByAnotherActiveWindow(t *testing.T) {
	db := openTestDB(t)
	s := NewMaintenanceStore(db)
	comps := NewStatusComponentStore(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	comp := &status.Component{CompositionMode: status.CompositionMatchAll, MatchAllType: "container", DisplayName: "API", Visible: true}
	_, err := comps.CreateComponent(ctx, comp)
	require.NoError(t, err)
	first, err := s.CreateMaintenance(ctx, &status.MaintenanceWindow{Title: "first", StartsAt: now, EndsAt: now.Add(time.Hour)}, []string{comp.ID})
	require.NoError(t, err)
	second, err := s.CreateMaintenance(ctx, &status.MaintenanceWindow{Title: "second", StartsAt: now, EndsAt: now.Add(time.Hour)}, []string{comp.ID})
	require.NoError(t, err)

	require.NoError(t, s.SetActive(ctx, first, true, nil))
	covered, err := s.CoveredByAnotherActiveWindow(ctx, comp.ID, first)
	require.NoError(t, err)
	assert.False(t, covered, "the window itself does not count")
	covered, err = s.CoveredByAnotherActiveWindow(ctx, comp.ID, second)
	require.NoError(t, err)
	assert.True(t, covered)

	require.NoError(t, s.SetActive(ctx, first, false, nil))
	covered, err = s.CoveredByAnotherActiveWindow(ctx, comp.ID, second)
	require.NoError(t, err)
	assert.False(t, covered, "an inactive window does not hold the component")
}

func TestStatusComponents_KeepTheOverrideBeforeMaintenance(t *testing.T) {
	db := openTestDB(t)
	comps := NewStatusComponentStore(db)
	ctx := context.Background()

	comp := &status.Component{CompositionMode: status.CompositionMatchAll, MatchAllType: "container", DisplayName: "API", Visible: true}
	_, err := comps.CreateComponent(ctx, comp)
	require.NoError(t, err)

	degraded, maint := status.StatusDegraded, status.StatusUnderMaint
	comp.StatusOverride = &maint
	comp.OverrideBeforeMaintenance = &degraded
	require.NoError(t, comps.UpdateComponent(ctx, comp))

	got, err := comps.GetComponent(ctx, comp.ID)
	require.NoError(t, err)
	require.NotNil(t, got.OverrideBeforeMaintenance)
	assert.Equal(t, status.StatusDegraded, *got.OverrideBeforeMaintenance)
	assert.Equal(t, status.StatusUnderMaint, *got.StatusOverride)

	got.OverrideBeforeMaintenance = nil
	require.NoError(t, comps.UpdateComponent(ctx, got))
	got, err = comps.GetComponent(ctx, comp.ID)
	require.NoError(t, err)
	assert.Nil(t, got.OverrideBeforeMaintenance)
}
