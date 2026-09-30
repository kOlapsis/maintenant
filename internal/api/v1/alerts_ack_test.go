// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type ackRecordingEscalator struct {
	mu    sync.Mutex
	acked []string
}

func (r *ackRecordingEscalator) EvaluateCycle(_ context.Context) error                  { return nil }
func (r *ackRecordingEscalator) OnAlertCreated(_ context.Context, _ *alert.Alert) error { return nil }
func (r *ackRecordingEscalator) OnAlertAcknowledged(_ context.Context, id string, _ alert.Acknowledgment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acked = append(r.acked, id)
	return nil
}
func (r *ackRecordingEscalator) OnAlertResolved(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (r *ackRecordingEscalator) OnEditionDowngraded(_ context.Context) error { return nil }

type ackFixture struct {
	db        *store.DB
	alerts    *store.AlertStoreImpl
	engine    *alert.Engine
	escalator *ackRecordingEscalator
	events    chan SSEEvent
	handler   *AlertHandler
}

func newAckFixture(t *testing.T) ackFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	alerts := store.NewAlertStore(db)
	broker := NewSSEBroker(logger)
	events := make(chan SSEEvent, 16)
	broker.AddObserver(events)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alerts,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
		Broadcaster:  alert.NewSSEBroadcasterFunc(broker.BroadcastEvent),
	})
	esc := &ackRecordingEscalator{}
	engine.SetEscalator(esc)
	return ackFixture{
		db:        db,
		alerts:    alerts,
		engine:    engine,
		escalator: esc,
		events:    events,
		handler:   NewAlertHandler(alerts, nil, nil, nil, broker, false, engine),
	}
}

func (f ackFixture) insert(t *testing.T, source, alertType, entityID string, firedAt time.Time) string {
	t.Helper()
	id, err := f.alerts.InsertAlert(context.Background(), &alert.Alert{
		Source: source, AlertType: alertType, Severity: alert.SeverityWarning, Status: alert.StatusActive,
		Message: "m", EntityType: "container", EntityID: entityID, EntityName: entityID, FiredAt: firedAt,
	})
	require.NoError(t, err)
	return id
}

func (f ackFixture) acknowledge(id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/"+id+"/acknowledge", strings.NewReader(`{"acknowledged_by":"alice"}`))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	f.handler.HandleAcknowledgeAlert(rec, req)
	return rec
}

func (f ackFixture) eventTypes() []string {
	var types []string
	for {
		select {
		case e := <-f.events:
			types = append(types, e.Type)
		default:
			return types
		}
	}
}

func TestHandleAcknowledgeAlert_SharedPath(t *testing.T) {
	f := newAckFixture(t)
	id := f.insert(t, alert.SourceContainer, "health_unhealthy", "c1", time.Now())

	rec := f.acknowledge(id)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got alert.Alert
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "alice", got.AcknowledgedBy)
	require.NotNil(t, got.AcknowledgedAt)
	assert.Equal(t, []string{event.AlertAcknowledged}, f.eventTypes())
	assert.Equal(t, []string{id}, f.escalator.acked)

	assert.Equal(t, http.StatusConflict, f.acknowledge(id).Code)
	assert.Equal(t, http.StatusNotFound, f.acknowledge("00000000-0000-0000-0000-00000000dead").Code)
	assert.Empty(t, f.eventTypes(), "a refused acknowledgment announces nothing")
}

func TestPostureAcknowledgment_AcknowledgesTheSecurityAlertThroughTheSharedPath(t *testing.T) {
	f := newAckFixture(t)
	ctx := context.Background()
	c := &container.Container{ID: "c1", ExternalID: "ext-1", Name: "web"}
	id := f.insert(t, alert.SourceSecurity, alert.AlertTypeDangerousConfig, c.ID, time.Now())

	securitySvc := security.NewService(security.Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	insight := security.Insight{Type: "privileged_container", Severity: "critical", ContainerID: c.ID, ContainerName: c.Name}
	securitySvc.UpdateContainer(c.ID, c.Name, []security.Insight{insight})
	acks := store.NewAcknowledgmentStore(f.db)
	_, err := acks.InsertAcknowledgment(ctx, &security.RiskAcknowledgment{
		ContainerExternalID: c.ExternalID, FindingType: string(insight.Type),
		FindingKey: security.InsightFindingKey(insight), AcknowledgedBy: "alice", AcknowledgedAt: time.Now(),
	})
	require.NoError(t, err)

	h := NewPostureHandler(nil, nil, acks, f.alerts, f.engine, securitySvc)
	h.tryAcknowledgeSecurityAlert(ctx, c, "alice")

	stored, err := f.alerts.GetAlert(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, stored.AcknowledgedAt)
	assert.Equal(t, []string{id}, f.escalator.acked, "accepting every risk must stop the escalation like any acknowledgment")
	assert.Equal(t, []string{event.AlertAcknowledged}, f.eventTypes())
}

func TestHandleListAlerts_PagesThroughAlertsFiredInTheSameSecond(t *testing.T) {
	f := newAckFixture(t)
	firedAt := time.Now().UTC().Truncate(time.Second)
	want := map[string]bool{}
	for i := range 3 {
		want[f.insert(t, alert.SourceContainer, "health_unhealthy", "c"+strconv.Itoa(i), firedAt)] = true
	}

	list := func(query url.Values) (int, []alert.Alert, bool) {
		rec := httptest.NewRecorder()
		f.handler.HandleListAlerts(rec, httptest.NewRequest(http.MethodGet, "/api/v1/alerts?"+query.Encode(), nil))
		var body struct {
			Alerts  []alert.Alert `json:"alerts"`
			HasMore bool          `json:"has_more"`
		}
		if rec.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		}
		return rec.Code, body.Alerts, body.HasMore
	}

	code, first, more := list(url.Values{"limit": {"2"}})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, first, 2)
	require.True(t, more)
	last := first[1]
	code, second, more := list(url.Values{"limit": {"2"}, "before": {last.FiredAt.Format(time.RFC3339)}, "before_id": {last.ID}})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, second, 1, "the alert fired in the same second as the last one listed must be on the next page")
	assert.False(t, more)

	got := map[string]bool{}
	for _, a := range append(first, second...) {
		got[a.ID] = true
	}
	assert.Equal(t, want, got)

	code, _, _ = list(url.Values{"before_id": {last.ID}})
	assert.Equal(t, http.StatusBadRequest, code, "before_id alone names no position")
}
