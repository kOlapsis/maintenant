// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/update"
	"github.com/kolapsis/maintenant/internal/webhook"
)

var (
	errOutage = &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	errSQL    = errors.New(`insert into status_components: SQLSTATE 42P01 relation "status_components" does not exist`)
	errUnique = &pgconn.PgError{Code: "23505", Message: `duplicate key value violates unique constraint "idx_name"`}
)

func call(h http.HandlerFunc, method, body string, pathValues ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	for i := 0; i+1 < len(pathValues); i += 2 {
		req.SetPathValue(pathValues[i], pathValues[i+1])
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestChannels_RenamingToATakenNameAnswersConflict(t *testing.T) {
	withEditionPinned(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	channels := store.NewChannelStore(storetest.Open(t, logger))
	h := &AlertHandler{notifier: channelNotifier(), channelStore: channels, broker: NewSSEBroker(logger), allowPrivateWebhooks: true}
	ctx := context.Background()
	_, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://hooks.example.com/a", Enabled: true})
	require.NoError(t, err)
	other := &alert.NotificationChannel{Name: "dev", Type: "webhook", URL: "https://hooks.example.com/b", Enabled: true}
	_, err = channels.InsertChannel(ctx, other)
	require.NoError(t, err)

	rec := call(h.HandleUpdateChannel, http.MethodPut, `{"name":"ops"}`, "id", other.ID)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.NotContains(t, strings.ToLower(rec.Body.String()), "unique")
}

type outageComponents struct{ status.ComponentStore }

func (outageComponents) GetComponent(context.Context, string) (*status.Component, error) {
	return nil, errOutage
}

type outageIncidents struct{ status.IncidentStore }

func (outageIncidents) GetIncident(context.Context, string) (*status.Incident, error) {
	return nil, errOutage
}

type outageMaintenance struct{ status.MaintenanceStore }

func (outageMaintenance) GetMaintenance(context.Context, string) (*status.MaintenanceWindow, error) {
	return nil, errOutage
}

func TestStatusAdmin_AStorageOutageIsNotNotFound(t *testing.T) {
	withEdition(t, extension.Pro)
	h := NewStatusAdminHandler(outageComponents{}, outageIncidents{}, nil, outageMaintenance{}, nil, nil, nil)
	cases := map[string]http.HandlerFunc{
		"update component":   h.HandleUpdateComponent,
		"post update":        h.HandlePostUpdate,
		"update incident":    h.HandleUpdateIncident,
		"update maintenance": h.HandleUpdateMaintenance,
	}
	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			rec := call(handler, http.MethodPut, `{}`, "id", "x")
			assert.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		})
	}
}

func TestStatusAdmin_AMissingObjectIsStillNotFound(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	h := NewStatusAdminHandler(store.NewStatusComponentStore(db), store.NewIncidentStore(db), nil, store.NewMaintenanceStore(db), nil, nil, nil)

	assert.Equal(t, http.StatusNotFound, call(h.HandleUpdateComponent, http.MethodPut, `{}`, "id", "missing").Code)
	assert.Equal(t, http.StatusNotFound, call(h.HandlePostUpdate, http.MethodPost, `{}`, "id", "missing").Code)
	assert.Equal(t, http.StatusNotFound, call(h.HandleUpdateIncident, http.MethodPut, `{}`, "id", "missing").Code)
	assert.Equal(t, http.StatusNotFound, call(h.HandleUpdateMaintenance, http.MethodPut, `{}`, "id", "missing").Code)
}

type failingIncidents struct {
	status.IncidentStore
	err error
}

func (f failingIncidents) CreateIncident(context.Context, *status.Incident, []string, string) (string, error) {
	return "", f.err
}

func (f failingIncidents) ListIncidents(context.Context, status.ListIncidentsOpts) ([]status.Incident, int, error) {
	return nil, 0, f.err
}

func TestStatusAdmin_StoreErrorsNeverReachTheClient(t *testing.T) {
	withEdition(t, extension.Pro)
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"sql failure", errSQL, http.StatusInternalServerError},
		{"unique violation", errUnique, http.StatusConflict},
		{"outage", errOutage, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewStatusAdminHandler(nil, failingIncidents{err: tc.err}, nil, nil, nil, nil, nil)

			for name, rec := range map[string]*httptest.ResponseRecorder{
				"create": call(h.HandleCreateIncident, http.MethodPost, `{"title":"t","severity":"minor"}`),
				"list":   call(h.HandleListIncidents, http.MethodGet, ""),
			} {
				assert.Equal(t, tc.code, rec.Code, name+": "+rec.Body.String())
				body := strings.ToLower(rec.Body.String())
				for _, leak := range []string{"sqlstate", "status_components", "duplicate key", "idx_name", "connection refused"} {
					assert.NotContains(t, body, leak, name)
				}
			}
		})
	}
}

type failingWebhooks struct {
	webhook.WebhookSubscriptionStore
	getErr error
	sub    *webhook.WebhookSubscription
}

func (f failingWebhooks) GetByID(context.Context, string) (*webhook.WebhookSubscription, error) {
	return f.sub, f.getErr
}

func (failingWebhooks) Delete(context.Context, string) error { return nil }

func TestWebhooks_DeleteTellsAnOutageFromAMissingWebhook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	outage := NewWebhookHandler(failingWebhooks{getErr: errOutage}, nil, logger, true)
	assert.Equal(t, http.StatusServiceUnavailable, call(outage.HandleDeleteWebhook, http.MethodDelete, "", "id", "w1").Code)

	missing := NewWebhookHandler(failingWebhooks{}, nil, logger, true)
	assert.Equal(t, http.StatusNotFound, call(missing.HandleDeleteWebhook, http.MethodDelete, "", "id", "w1").Code)

	present := NewWebhookHandler(failingWebhooks{sub: &webhook.WebhookSubscription{ID: "w1"}}, nil, logger, true)
	assert.Equal(t, http.StatusNoContent, call(present.HandleDeleteWebhook, http.MethodDelete, "", "id", "w1").Code)
}

type pinStore struct {
	update.UpdateStore
	updateErr error
	deleteErr error
}

func (pinStore) GetImageUpdateByContainer(context.Context, string) (*update.ImageUpdate, error) {
	return &update.ImageUpdate{ContainerID: "c1", Image: "nginx", CurrentTag: "1.0"}, nil
}
func (pinStore) InsertVersionPin(context.Context, *update.VersionPin) (string, error) {
	return "p", nil
}
func (p pinStore) UpdateImageUpdate(context.Context, *update.ImageUpdate) error { return p.updateErr }
func (p pinStore) DeleteVersionPin(context.Context, string) error               { return p.deleteErr }

func TestUpdates_PinAndUnpinReportAFailedStatusWrite(t *testing.T) {
	h := NewUpdateHandler(nil, pinStore{updateErr: errSQL}, nil)

	pin := call(h.HandlePinVersion, http.MethodPost, `{}`, "container_id", "c1")
	assert.Equal(t, http.StatusInternalServerError, pin.Code, pin.Body.String())
	assert.NotContains(t, pin.Body.String(), "SQLSTATE")

	unpin := call(h.HandleUnpinVersion, http.MethodDelete, "", "container_id", "c1")
	assert.Equal(t, http.StatusInternalServerError, unpin.Code, unpin.Body.String())

	ok := NewUpdateHandler(nil, pinStore{}, nil)
	assert.Equal(t, http.StatusOK, call(ok.HandlePinVersion, http.MethodPost, `{}`, "container_id", "c1").Code)
	assert.Equal(t, http.StatusNoContent, call(ok.HandleUnpinVersion, http.MethodDelete, "", "container_id", "c1").Code)
}

func TestMapServiceError_TranslatesStoreFailures(t *testing.T) {
	for err, code := range map[error]int{errOutage: http.StatusServiceUnavailable, errUnique: http.StatusConflict, errSQL: http.StatusInternalServerError} {
		rec := httptest.NewRecorder()
		mapServiceError(rec, err)
		assert.Equal(t, code, rec.Code)
		assert.NotContains(t, strings.ToLower(rec.Body.String()), "sqlstate")
	}
}
