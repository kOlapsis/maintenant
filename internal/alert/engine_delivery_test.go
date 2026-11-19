// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type insertFailsOnce struct {
	alert.ChannelStore
	failed atomic.Bool
}

func (s *insertFailsOnce) InsertDelivery(ctx context.Context, d *alert.NotificationDelivery) (string, error) {
	if s.failed.CompareAndSwap(false, true) {
		return "", errors.New("database is locked")
	}
	return s.ChannelStore.InsertDelivery(ctx, d)
}

type payloadServer struct {
	*httptest.Server
	mu       sync.Mutex
	payloads []alert.WebhookPayload
}

func newPayloadServer(t *testing.T) *payloadServer {
	t.Helper()
	s := &payloadServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p alert.WebhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.mu.Lock()
		s.payloads = append(s.payloads, p)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *payloadServer) received() []alert.WebhookPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]alert.WebhookPayload(nil), s.payloads...)
}

type deliveryEngine struct {
	ctx      context.Context
	logger   *slog.Logger
	alerts   alert.AlertStore
	channels *store.ChannelStoreImpl
	engine   *alert.Engine
}

// newDeliveryEngine runs an engine whose single trigger sends every alert, recoveries included, to url.
func newDeliveryEngine(t *testing.T, url string, engineChannels func(*store.ChannelStoreImpl) alert.ChannelStore, notifier func(*store.ChannelStoreImpl, *slog.Logger) *alert.Notifier) *deliveryEngine {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	triggers := store.NewTriggerStore(db)
	chID, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "hook", Type: "webhook", URL: url, Enabled: true})
	require.NoError(t, err)
	_, err = triggers.InsertTrigger(ctx, &alert.AlertTrigger{Name: "all", Enabled: true, NotifyOnResolve: true, ChannelIDs: []string{chID}})
	require.NoError(t, err)

	e := &deliveryEngine{ctx: ctx, logger: logger, alerts: store.NewAlertStore(db), channels: channels}
	e.engine = alert.NewEngine(alert.EngineDeps{
		AlertStore:   e.alerts,
		ChannelStore: engineChannels(channels),
		TriggerStore: triggers,
		SilenceStore: store.NewSilenceStore(db),
		Notifier:     notifier(channels, logger),
		Logger:       logger,
	})
	e.engine.Start(ctx)
	return e
}

func (e *deliveryEngine) emit(severity, message string, recover bool) {
	e.engine.EventChannel() <- alert.Event{
		Source: "endpoint", AlertType: "down", Severity: severity, IsRecover: recover, Message: message,
		EntityType: "endpoint", EntityID: "e1", EntityName: "api", Timestamp: time.Now(),
	}
}

func (e *deliveryEngine) firedID(t *testing.T) string {
	t.Helper()
	var id string
	require.Eventually(t, func() bool {
		a, err := e.alerts.GetActiveAlert(e.ctx, "endpoint", "down", "endpoint", "e1")
		if err != nil || a == nil {
			return false
		}
		id = a.ID
		return true
	}, 10*time.Second, 5*time.Millisecond)
	return id
}

func (e *deliveryEngine) deliveries(alertID string) []*alert.NotificationDelivery {
	ds, err := e.channels.ListDeliveriesByAlert(e.ctx, alertID)
	if err != nil {
		return nil
	}
	return ds
}

func TestEngineDelivery_SentWhenItsRowFailsToInsertOnce(t *testing.T) {
	srv := newPayloadServer(t)
	e := newDeliveryEngine(t, srv.URL,
		func(c *store.ChannelStoreImpl) alert.ChannelStore { return &insertFailsOnce{ChannelStore: c} },
		func(c *store.ChannelStoreImpl, logger *slog.Logger) *alert.Notifier {
			n := alert.NewNotifier(c, logger, true)
			n.StartUntilCleanup(context.Background(), t)
			return n
		})

	e.emit(alert.SeverityCritical, "api down", false)
	id := e.firedID(t)

	require.Eventually(t, func() bool { return len(srv.received()) == 1 }, 10*time.Second, 5*time.Millisecond,
		"the delivery row is written on the second try and the notification goes out")
	require.Eventually(t, func() bool {
		ds := e.deliveries(id)
		return len(ds) == 1 && ds[0].Status == alert.DeliveryDelivered
	}, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, "api down", srv.received()[0].Alert["message"])
}

func TestEngineDelivery_ARestartSendsExactlyWhatWasDecided(t *testing.T) {
	srv := newPayloadServer(t)
	e := newDeliveryEngine(t, srv.URL,
		func(c *store.ChannelStoreImpl) alert.ChannelStore { return c },
		// Never started: the process stops before any notification leaves.
		func(c *store.ChannelStoreImpl, logger *slog.Logger) *alert.Notifier {
			return alert.NewNotifier(c, logger, true)
		})

	e.emit(alert.SeverityWarning, "api slow", false)
	id := e.firedID(t)
	e.emit(alert.SeverityCritical, "api down", false)
	e.emit(alert.SeverityInfo, "api back", true)
	require.Eventually(t, func() bool { return len(e.deliveries(id)) == 3 }, 10*time.Second, 5*time.Millisecond)

	stored := e.deliveries(id)
	types := []string{}
	for _, d := range stored {
		types = append(types, d.EventType)
		assert.Equal(t, alert.DeliveryPending, d.Status)
	}
	assert.Equal(t, []string{alert.EventFired, alert.EventSeverityRaised, alert.EventResolved}, types)

	restarted := alert.NewNotifier(e.channels, e.logger, true)
	restarted.SetResumeInterval(5 * time.Millisecond)
	restarted.StartUntilCleanup(e.ctx, t)

	require.Eventually(t, func() bool { return len(srv.received()) == 3 }, 10*time.Second, 5*time.Millisecond)
	type sent struct{ Event, Status, Severity, Message string }
	var got []sent
	for _, p := range srv.received() {
		got = append(got, sent{p.Event, p.Alert["status"].(string), p.Alert["severity"].(string), p.Alert["message"].(string)})
		assert.Equal(t, id, p.Alert["id"])
	}
	assert.Equal(t, []sent{
		{"alert.fired", alert.StatusActive, alert.SeverityWarning, "api slow"},
		{"alert.fired", alert.StatusActive, alert.SeverityCritical, "Severity raised from warning to critical: api down"},
		{"alert.resolved", alert.StatusResolved, alert.SeverityInfo, "api back"},
	}, got, "each notification as it was when decided, in order, though the alert has changed since")
}
