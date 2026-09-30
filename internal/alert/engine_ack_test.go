// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type recordingEscalator struct {
	mu      sync.Mutex
	created int
	acked   []string
}

func (r *recordingEscalator) EvaluateCycle(_ context.Context) error { return nil }
func (r *recordingEscalator) OnAlertCreated(_ context.Context, _ *alert.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	return nil
}
func (r *recordingEscalator) OnAlertAcknowledged(_ context.Context, id string, _ alert.Acknowledgment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acked = append(r.acked, id)
	return nil
}
func (r *recordingEscalator) OnAlertResolved(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (r *recordingEscalator) OnEditionDowngraded(_ context.Context) error { return nil }

func (r *recordingEscalator) createdCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created
}

type recordingBroadcaster struct {
	mu     sync.Mutex
	events []string
}

func (b *recordingBroadcaster) Broadcast(eventType string, _ interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, eventType)
}

func (b *recordingBroadcaster) count(eventType string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, e := range b.events {
		if e == eventType {
			n++
		}
	}
	return n
}

type ackFixture struct {
	alerts      alert.AlertStore
	channels    alert.ChannelStore
	triggers    alert.TriggerStore
	engine      *alert.Engine
	escalator   *recordingEscalator
	broadcaster *recordingBroadcaster
}

func newAckFixture(t *testing.T, ctx context.Context) ackFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	f := ackFixture{
		alerts:      store.NewAlertStore(db),
		channels:    store.NewChannelStore(db),
		triggers:    store.NewTriggerStore(db),
		escalator:   &recordingEscalator{},
		broadcaster: &recordingBroadcaster{},
	}
	notifier := alert.NewNotifier(f.channels, logger, true)
	notifier.Start(ctx)
	f.engine = alert.NewEngine(alert.EngineDeps{
		AlertStore:   f.alerts,
		ChannelStore: f.channels,
		TriggerStore: f.triggers,
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
		Notifier:     notifier,
		Broadcaster:  f.broadcaster,
	})
	f.engine.SetEscalator(f.escalator)
	f.engine.Start(ctx)
	return f
}

func unhealthyEvent(severity, message string) alert.Event {
	return alert.Event{
		Source:     alert.SourceContainer,
		AlertType:  "health_unhealthy",
		Severity:   severity,
		EntityType: "container",
		EntityID:   "c1",
		EntityName: "web",
		Message:    message,
		Timestamp:  time.Now(),
	}
}

func TestEngineAcknowledge_BroadcastsAndStopsEscalation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newAckFixture(t, ctx)

	id, err := f.alerts.InsertAlert(ctx, &alert.Alert{
		Source: alert.SourceContainer, AlertType: "health_unhealthy", Severity: alert.SeverityWarning,
		Status: alert.StatusActive, Message: "unhealthy", EntityType: "container", EntityID: "c1",
		EntityName: "web", FiredAt: time.Now(),
	})
	require.NoError(t, err)

	a, err := f.engine.Acknowledge(ctx, id, "alice")
	require.NoError(t, err)
	require.NotNil(t, a.AcknowledgedAt)
	assert.Equal(t, "alice", a.AcknowledgedBy)
	assert.Equal(t, 1, f.broadcaster.count(event.AlertAcknowledged))
	assert.Equal(t, []string{id}, f.escalator.acked)

	stored, err := f.alerts.GetAlert(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, stored.AcknowledgedAt)
	assert.Equal(t, "alice", stored.AcknowledgedBy)

	_, err = f.engine.Acknowledge(ctx, id, "bob")
	require.ErrorIs(t, err, alert.ErrNotAcknowledgeable)
	assert.Equal(t, 1, f.broadcaster.count(event.AlertAcknowledged), "a refused acknowledgment announces nothing")

	_, err = f.engine.Acknowledge(ctx, "00000000-0000-0000-0000-00000000dead", "bob")
	require.ErrorIs(t, err, alert.ErrAlertNotFound)
}

func TestEngineSeverityRaise_AcknowledgedAlertIsNotifiedOnceAndNeverEscalated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newAckFixture(t, ctx)

	srv, bodies := capturingWebhookServer(t)
	chID, err := f.channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: srv.URL, Enabled: true})
	require.NoError(t, err)
	seedTriggerForChannel(t, f.triggers, "all", true, "", "", []string{chID})

	f.engine.EventChannel() <- unhealthyEvent(alert.SeverityWarning, "Container web is unhealthy")
	require.Eventually(t, func() bool { return len(bodies()) == 1 }, 5*time.Second, 10*time.Millisecond)
	active, err := f.alerts.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	id := active[0].ID

	_, err = f.engine.Acknowledge(ctx, id, "alice")
	require.NoError(t, err)

	f.engine.EventChannel() <- unhealthyEvent(alert.SeverityCritical, "Container web keeps failing")
	require.Eventually(t, func() bool { return len(bodies()) == 2 }, 5*time.Second, 10*time.Millisecond)
	require.Never(t, func() bool { return len(bodies()) > 2 }, 300*time.Millisecond, 20*time.Millisecond)

	raised, ok := bodies()[1]["alert"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Severity raised from warning to critical: Container web keeps failing", raised["message"])
	assert.Equal(t, alert.SeverityCritical, raised["severity"])
	assert.Equal(t, "alice", raised["acknowledged_by"], "the raise must not read as a fresh, unacknowledged alert")

	assert.Equal(t, 1, f.escalator.createdCount(), "an acknowledged alert must never start an escalation again")

	stored, err := f.alerts.GetAlert(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, alert.SeverityCritical, stored.Severity)
	require.NotNil(t, stored.AcknowledgedAt, "the raise must keep the acknowledgment")
	assert.Equal(t, "alice", stored.AcknowledgedBy)
}

func TestEngineSeverityRaise_UnacknowledgedAlertReevaluatesEscalation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newAckFixture(t, ctx)

	f.engine.EventChannel() <- unhealthyEvent(alert.SeverityWarning, "Container web is unhealthy")
	require.Eventually(t, func() bool { return f.escalator.createdCount() == 1 }, 5*time.Second, 10*time.Millisecond)

	f.engine.EventChannel() <- unhealthyEvent(alert.SeverityCritical, "Container web keeps failing")
	require.Eventually(t, func() bool { return f.escalator.createdCount() == 2 }, 5*time.Second, 10*time.Millisecond,
		"a raise on an unacknowledged alert may match policies the lower severity did not")
}
