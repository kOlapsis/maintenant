// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestStatusAdminWrites_ShowOnThePublicSnapshotAtOnce(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components, incidents, maintenance := store.NewStatusComponentStore(db), store.NewIncidentStore(db), store.NewMaintenanceStore(db)
	svc := status.NewService(status.Deps{Components: components, Logger: logger, Incidents: incidents, Maintenance: maintenance})
	admin := NewStatusAdminHandler(components, incidents, nil, maintenance, nil, svc, nil)
	public := status.NewHandler(svc, nil, logger, nil, "https://status.example.com")

	snapshot := func() status.StatusAPIResponse {
		rec := httptest.NewRecorder()
		public.HandleStatusAPI(rec, httptest.NewRequest(http.MethodGet, "/status/api", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var body status.StatusAPIResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body
	}
	call := func(h http.HandlerFunc, method, id, body string, want int) {
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h(rec, req)
		require.Equal(t, want, rec.Code, rec.Body.String())
	}

	incID, err := incidents.CreateIncident(ctx, &status.Incident{Title: "Database down", Severity: status.SeverityMajor, Status: status.IncidentInvestigating}, nil, "")
	require.NoError(t, err)
	require.Len(t, snapshot().ActiveIncidents, 1)

	call(admin.HandleUpdateIncident, http.MethodPut, incID, `{"title":"Database slow"}`, http.StatusOK)
	assert.Equal(t, "Database slow", snapshot().ActiveIncidents[0].Title, "incident update")

	call(admin.HandleDeleteIncident, http.MethodDelete, incID, "", http.StatusNoContent)
	assert.Empty(t, snapshot().ActiveIncidents, "incident deletion")

	starts := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	window := `{"title":"Upgrade","starts_at":"` + starts.Format(time.RFC3339) + `","ends_at":"` + starts.Add(time.Hour).Format(time.RFC3339) + `"}`
	call(admin.HandleCreateMaintenance, http.MethodPost, "", window, http.StatusCreated)
	upcoming := snapshot().UpcomingMaint
	require.Len(t, upcoming, 1, "maintenance creation")

	call(admin.HandleUpdateMaintenance, http.MethodPut, upcoming[0].ID, `{"title":"Upgrade, postponed"}`, http.StatusOK)
	assert.Equal(t, "Upgrade, postponed", snapshot().UpcomingMaint[0].Title, "maintenance update")

	call(admin.HandleDeleteMaintenance, http.MethodDelete, upcoming[0].ID, "", http.StatusNoContent)
	assert.Empty(t, snapshot().UpcomingMaint, "maintenance deletion")
}
