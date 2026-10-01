// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/statuspage"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestStatusPageWriteRoutes_PerEdition(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	components := store.NewStatusComponentStore(db)
	r := NewRouter(HandlerDeps{
		Logger:             logger,
		StatusComponents:   components,
		StatusIncidents:    store.NewIncidentStore(db),
		StatusMaintenance:  store.NewMaintenanceStore(db),
		StatusSubscribers:  store.NewSubscriberStore(db),
		StatusSvc:          status.NewService(status.Deps{Components: components, Logger: logger}),
		StatusMailer:       newRecordingMailer(),
		PersonalizationSvc: statuspage.NewPersonalizationService(store.NewPersonalizationStore(db), logger),
	})
	h := r.Handler()

	type route struct {
		capability     extension.Capability
		method, path   string
		body           string
		successfulCode int
	}
	routes := []route{
		{extension.CapIncidents, http.MethodPost, "/api/v1/status/incidents", `{"title":"db down","severity":"minor"}`, http.StatusCreated},
		{extension.CapSMTP, http.MethodPost, "/api/v1/status/smtp/test", `{"to":"ops@example.com"}`, http.StatusOK},
		{extension.CapMaintenanceWindows, http.MethodPost, "/api/v1/status/maintenance", `{"title":"upgrade","starts_at":"2030-01-01T00:00:00Z","ends_at":"2030-01-01T01:00:00Z"}`, http.StatusCreated},
		{extension.CapSubscribers, http.MethodGet, "/api/v1/status/subscribers", "", http.StatusOK},
		{extension.CapPersonalization, http.MethodGet, "/api/v1/status-page/footer-links", "", http.StatusOK},
	}
	opens := map[extension.Edition]map[extension.Capability]bool{
		extension.Community: {},
		extension.Personal:  {extension.CapIncidents: true, extension.CapSMTP: true},
		extension.Pro: {
			extension.CapIncidents: true, extension.CapSMTP: true, extension.CapMaintenanceWindows: true,
			extension.CapSubscribers: true, extension.CapPersonalization: true,
		},
	}

	for edition, open := range opens {
		for _, rt := range routes {
			t.Run(string(edition)+" "+rt.method+" "+rt.path, func(t *testing.T) {
				withEdition(t, edition)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body)))

				if !open[rt.capability] {
					require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
					detail := decodeRefusal(t, rec)
					assert.Equal(t, "EDITION_REQUIRED", detail.Code)
					assert.Equal(t, string(rt.capability), detail.Feature)
					return
				}
				assert.Equal(t, rt.successfulCode, rec.Code, rec.Body.String())
			})
		}
	}
}
