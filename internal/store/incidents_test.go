// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

func TestIncidentAndMaintenanceComponentsCarryTheirVisibility(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	components := NewStatusComponentStore(db)
	visible := map[string]bool{}
	var ids []string
	for _, c := range []*status.Component{
		{CompositionMode: status.CompositionMatchAll, MatchAllType: "endpoint", DisplayName: "API", Visible: true},
		{CompositionMode: status.CompositionMatchAll, MatchAllType: "container", DisplayName: "Internal DB"},
	} {
		id, err := components.CreateComponent(ctx, c)
		require.NoError(t, err)
		ids = append(ids, id)
		visible[id] = c.Visible
	}

	incidentID, err := NewIncidentStore(db).CreateIncident(ctx, &status.Incident{
		Title: "Slow", Severity: status.SeverityMinor, Status: status.IncidentInvestigating,
	}, ids, "")
	require.NoError(t, err)
	inc, err := NewIncidentStore(db).GetIncident(ctx, incidentID)
	require.NoError(t, err)

	now := time.Now().UTC()
	windowID, err := NewMaintenanceStore(db).CreateMaintenance(ctx, &status.MaintenanceWindow{
		Title: "Upgrade", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour),
	}, ids)
	require.NoError(t, err)
	mw, err := NewMaintenanceStore(db).GetMaintenance(ctx, windowID)
	require.NoError(t, err)

	for name, refs := range map[string][]status.IncidentCompRef{"incident": inc.Components, "maintenance": mw.Components} {
		require.Len(t, refs, 2, name)
		for _, ref := range refs {
			assert.Equal(t, visible[ref.ID], ref.Visible, "%s component %s", name, ref.Name)
		}
	}
}
