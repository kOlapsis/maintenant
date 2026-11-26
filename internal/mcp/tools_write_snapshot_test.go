// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestStatusWriteTools_ShowOnThePublicSnapshotAtOnce(t *testing.T) {
	pinEdition(t, extension.Pro)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components, incidents, maintenance := store.NewStatusComponentStore(db), store.NewIncidentStore(db), store.NewMaintenanceStore(db)
	statusSvc := status.NewService(status.Deps{
		Components: components, Logger: logger, Incidents: incidents, Maintenance: maintenance,
		PublicBroadcaster: func(string, any) {},
	})
	svc := &Services{StatusComponents: components, Incidents: incidents, Maintenance: maintenance, IncidentAnnouncer: statusSvc, Logger: logger}
	snapshot := func() *status.PageData {
		data, err := statusSvc.PublicPageData(ctx)
		require.NoError(t, err)
		return data
	}
	require.Empty(t, snapshot().Maintenance)

	starts := time.Now().Add(time.Hour).UTC()
	res, _, err := createMaintenanceHandler(svc)(ctx, nil, createMaintenanceInput{
		Title: "Upgrade", StartTime: starts.Format(time.RFC3339), EndTime: starts.Add(time.Hour).Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.False(t, res.IsError, textFromContent(t, res.Content))
	require.Len(t, snapshot().Maintenance, 1, "create_maintenance")

	res, _, err = createIncidentHandler(svc)(ctx, nil, createIncidentInput{Title: "Database down", Severity: status.SeverityMajor})
	require.NoError(t, err)
	require.False(t, res.IsError, textFromContent(t, res.Content))
	active := snapshot().ActiveIncidents
	require.Len(t, active, 1, "create_incident")

	res, _, err = updateIncidentHandler(svc)(ctx, nil, updateIncidentInput{IncidentID: active[0].ID, Status: status.IncidentResolved, Message: "fixed"})
	require.NoError(t, err)
	require.False(t, res.IsError, textFromContent(t, res.Content))
	require.Empty(t, snapshot().ActiveIncidents, "update_incident")
}
