// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestStatusWrites_RefuseComponentIDsWithoutWritingAnything(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	incidents := store.NewIncidentStore(db)
	maintenance := store.NewMaintenanceStore(db)
	svc := &Services{Incidents: incidents, Maintenance: maintenance, StatusComponents: components, Logger: logger, Version: "test"}
	ctx := context.Background()

	comp := &status.Component{CompositionMode: status.CompositionMatchAll, MatchAllType: "endpoint", DisplayName: "API", Visible: true}
	_, err := components.CreateComponent(ctx, comp)
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		ids  []string
		want string
	}{
		"empty":    {[]string{comp.ID, ""}, `component_ids[1] is empty`},
		"unknown":  {[]string{"00000000-0000-0000-0000-00000000dead"}, `component_ids[0]: no component "00000000-0000-0000-0000-00000000dead"`},
		"repeated": {[]string{comp.ID, comp.ID}, `component_ids[1] repeats "` + comp.ID + `"`},
	} {
		t.Run(name, func(t *testing.T) {
			result, _, err := createIncidentHandler(svc)(ctx, nil, createIncidentInput{
				Title: "down", Severity: status.SeverityMajor, ComponentIDs: tc.ids,
			})
			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Contains(t, textFromContent(t, result.Content), tc.want)

			result, _, err = createMaintenanceHandler(svc)(ctx, nil, createMaintenanceInput{
				Title: "db upgrade", StartTime: "2099-01-01T02:00:00Z", EndTime: "2099-01-01T04:00:00Z", ComponentIDs: tc.ids,
			})
			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Contains(t, textFromContent(t, result.Content), tc.want)
		})
	}

	_, total, err := incidents.ListIncidents(ctx, status.ListIncidentsOpts{})
	require.NoError(t, err)
	assert.Zero(t, total, "no incident is left without its components")
	windows, err := maintenance.ListMaintenance(ctx, "", 20)
	require.NoError(t, err)
	assert.Empty(t, windows, "no maintenance window is left without its components")

	result, _, err := createMaintenanceHandler(svc)(ctx, nil, createMaintenanceInput{
		Title: "db upgrade", StartTime: "2099-01-01T02:00:00Z", EndTime: "2099-01-01T04:00:00Z", ComponentIDs: []string{comp.ID},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError, textFromContent(t, result.Content))
}
