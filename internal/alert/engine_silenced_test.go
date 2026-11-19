// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial/maintenance"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

const fastRecheck = 10 * time.Millisecond

type createdRecorder struct {
	mu  sync.Mutex
	ids []string
}

func (r *createdRecorder) EvaluateCycle(context.Context) error { return nil }
func (r *createdRecorder) OnAlertCreated(_ context.Context, a *alert.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, a.ID)
	return nil
}
func (r *createdRecorder) OnAlertAcknowledged(context.Context, string, alert.Acknowledgment) error {
	return nil
}
func (r *createdRecorder) OnAlertResolved(context.Context, string, time.Time) error { return nil }
func (r *createdRecorder) OnEditionDowngraded(context.Context) error                { return nil }

func (r *createdRecorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, got := range r.ids {
		if got == id {
			n++
		}
	}
	return n
}

// silenceFixture runs engines over one real database whose single trigger notifies every alert.
type silenceFixture struct {
	ctx       context.Context
	db        *store.DB
	logger    *slog.Logger
	alerts    *store.AlertStoreImpl
	channels  *store.ChannelStoreImpl
	triggers  *store.TriggerStoreImpl
	silences  *store.SilenceStoreImpl
	escalator *createdRecorder
	engine    *alert.Engine
	stop      context.CancelFunc
	sentinels int
}

func newSilenceFixture(t *testing.T) *silenceFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	f := &silenceFixture{
		ctx:       context.Background(),
		db:        db,
		logger:    logger,
		alerts:    store.NewAlertStore(db),
		channels:  store.NewChannelStore(db),
		triggers:  store.NewTriggerStore(db),
		silences:  store.NewSilenceStore(db),
		escalator: &createdRecorder{},
	}
	chID, err := f.channels.InsertChannel(f.ctx, &alert.NotificationChannel{Name: "hook", Type: "webhook", URL: "https://example.com/hook", Enabled: true})
	require.NoError(t, err)
	_, err = f.triggers.InsertTrigger(f.ctx, &alert.AlertTrigger{
		Name: "all", Enabled: true, NotifyOnResolve: true, ChannelIDs: []string{chID},
	})
	require.NoError(t, err)
	t.Cleanup(f.halt)
	return f
}

// start runs a fresh engine over the database, as a process start does, stopping the previous one.
func (f *silenceFixture) start(recheck time.Duration) {
	f.halt()
	ctx, cancel := context.WithCancel(f.ctx)
	f.stop = cancel
	eng := alert.NewEngine(alert.EngineDeps{
		AlertStore:   f.alerts,
		ChannelStore: f.channels,
		TriggerStore: f.triggers,
		SilenceStore: f.silences,
		// Never started: the delivery row is the notification, and no send outlives the test.
		Notifier: alert.NewNotifier(f.channels, f.logger, true),
		Logger:   f.logger,
	})
	eng.SetEscalator(f.escalator)
	eng.SetMaintenanceSuppressor(maintenance.NewSuppressor(store.NewMaintenanceStore(f.db), f.logger))
	eng.SetSilenceRecheckInterval(recheck)
	eng.Start(ctx)
	f.engine = eng
}

func (f *silenceFixture) halt() {
	if f.stop != nil {
		f.stop()
		f.stop = nil
	}
}

func (f *silenceFixture) send(evt alert.Event) { f.engine.EventChannel() <- evt }

// barrier returns once the engine has processed every event sent before it.
func (f *silenceFixture) barrier(t *testing.T) {
	t.Helper()
	id := fmt.Sprintf("sentinel-%s", uid.New())
	f.sentinels++
	f.send(alert.Event{
		Source: sentinelSource, AlertType: "barrier", Severity: alert.SeverityInfo,
		EntityType: "sentinel", EntityID: id, EntityName: id, Timestamp: time.Now(),
	})
	require.Eventually(t, func() bool {
		a, err := f.alerts.GetActiveAlert(f.ctx, sentinelSource, "barrier", "sentinel", id)
		return err == nil && a != nil
	}, 10*time.Second, 5*time.Millisecond, "the engine must get past the events sent before the barrier")
}

func (f *silenceFixture) silence(t *testing.T, rule alert.SilenceRule) {
	t.Helper()
	rule.StartsAt = time.Now().Add(-time.Minute)
	rule.DurationSeconds = 3600
	_, err := f.silences.InsertSilenceRule(f.ctx, &rule)
	require.NoError(t, err)
}

func (f *silenceFixture) silenceEndpoint(t *testing.T, entityID string) {
	t.Helper()
	f.silence(t, alert.SilenceRule{Source: "test", EntityType: "endpoint", EntityID: &entityID})
}

// maintain opens a one-hour maintenance window over an endpoint monitor.
func (f *silenceFixture) maintain(t *testing.T, entityID string) {
	t.Helper()
	now := time.Now()
	w := f.db.Writer()
	componentID, windowID := uid.New(), uid.New()
	_, err := w.Exec(f.ctx,
		`INSERT INTO status_components (id, composition_mode, match_all_type, display_name, display_order, visible, created_at, updated_at)
		 VALUES (?, 'explicit', NULL, 'Test', 0, 1, ?, ?)`, componentID, now.Unix(), now.Unix())
	require.NoError(t, err)
	_, err = w.Exec(f.ctx,
		`INSERT INTO maintenance_windows (id, title, description, starts_at, ends_at, active, created_at, updated_at)
		 VALUES (?, 'Test', '', ?, ?, 0, ?, ?)`, windowID, now.Add(-time.Minute).Unix(), now.Add(time.Hour).Unix(), now.Unix(), now.Unix())
	require.NoError(t, err)
	_, err = w.Exec(f.ctx, `INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`, windowID, componentID)
	require.NoError(t, err)
	_, err = w.Exec(f.ctx,
		`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, 'endpoint', ?)`, componentID, entityID)
	require.NoError(t, err)
}

// elapse moves every silence and maintenance window d into the past, as if the clock had moved forward.
func (f *silenceFixture) elapse(t *testing.T, d time.Duration) {
	t.Helper()
	secs := int64(d / time.Second)
	_, err := f.db.Writer().Exec(f.ctx, `UPDATE silence_rules SET starts_at = starts_at - ?`, secs)
	require.NoError(t, err)
	_, err = f.db.Writer().Exec(f.ctx, `UPDATE maintenance_windows SET starts_at = starts_at - ?, ends_at = ends_at - ?`, secs, secs)
	require.NoError(t, err)
}

func (f *silenceFixture) rows(t *testing.T, source, entityID string) []*alert.Alert {
	t.Helper()
	all, err := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: source, Limit: 200})
	require.NoError(t, err)
	var out []*alert.Alert
	for _, a := range all {
		if a.EntityID == entityID {
			out = append(out, a)
		}
	}
	return out
}

// held returns the single silenced row of an endpoint.
func (f *silenceFixture) held(t *testing.T, entityID string) *alert.Alert {
	t.Helper()
	rows := f.rows(t, "test", entityID)
	require.Len(t, rows, 1, "the outage must be stored once")
	require.Equal(t, alert.StatusSilenced, rows[0].Status)
	return rows[0]
}

func (f *silenceFixture) status(t *testing.T, id string) string {
	t.Helper()
	a, err := f.alerts.GetAlert(f.ctx, id)
	require.NoError(t, err)
	require.NotNil(t, a)
	return a.Status
}

func (f *silenceFixture) deliveries(t *testing.T, alertID string) int {
	t.Helper()
	ds, err := f.channels.ListDeliveriesByAlert(f.ctx, alertID)
	require.NoError(t, err)
	return len(ds)
}

// assertPromotedOnce waits for a held alert to go live, then checks it is notified and escalated exactly once.
func (f *silenceFixture) assertPromotedOnce(t *testing.T, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return f.status(t, id) == alert.StatusActive && f.deliveries(t, id) == 1 && f.escalator.count(id) == 1
	}, 10*time.Second, 5*time.Millisecond, "the held alert must go live, notified and escalated, once the window ends")
	assert.Never(t, func() bool {
		return f.deliveries(t, id) > 1 || f.escalator.count(id) > 1
	}, 200*time.Millisecond, fastRecheck, "a promoted alert must be notified once")
	assert.Equal(t, 1, f.engine.AlertCount()-f.sentinels, "the promoted alert must be live in memory")
}

func (f *silenceFixture) assertNeverNotified(t *testing.T, id string) {
	t.Helper()
	assert.Never(t, func() bool {
		return f.deliveries(t, id) > 0 || f.escalator.count(id) > 0
	}, 200*time.Millisecond, fastRecheck, "the alert must never be notified")
}

func TestSilenced_PromotedWhenTheSilenceEnds(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-1")
	f.start(fastRecheck)

	f.send(outageEvent("ep-1", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-1")
	assert.Zero(t, f.deliveries(t, held.ID), "nothing may be notified during the silence")

	f.elapse(t, 2*time.Hour)
	f.assertPromotedOnce(t, held.ID)
	assert.Len(t, f.rows(t, "test", "ep-1"), 1, "the promotion must reuse the held row")
}

func TestSilenced_PromotedWhenTheMaintenanceEnds(t *testing.T) {
	f := newSilenceFixture(t)
	f.maintain(t, "ep-2")
	f.start(fastRecheck)

	f.send(outageEvent("ep-2", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-2")
	assert.Zero(t, f.deliveries(t, held.ID), "nothing may be notified during the maintenance")

	f.elapse(t, 2*time.Hour)
	f.assertPromotedOnce(t, held.ID)
}

func TestSilenced_RepeatedEventsKeepOneRow(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-3")
	f.start(time.Hour)

	for range 5 {
		f.send(outageEvent("ep-3", alert.SeverityCritical, false))
	}
	f.barrier(t)

	f.held(t, "ep-3")
}

func TestSilenced_RecoveryDuringTheWindowResolvesQuietly(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-4")
	f.start(fastRecheck)

	f.send(outageEvent("ep-4", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-4")

	f.send(outageEvent("ep-4", alert.SeverityInfo, true))
	f.barrier(t)
	assert.Equal(t, alert.StatusResolved, f.status(t, held.ID), "the recovery must close the held row")

	f.elapse(t, 2*time.Hour)
	f.assertNeverNotified(t, held.ID)
	assert.Equal(t, alert.StatusResolved, f.status(t, held.ID))
	assert.Equal(t, 0, f.engine.AlertCount()-f.sentinels)
}

func TestSilenced_UnsilencedEventPromotesTheHeldRow(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-5")
	f.start(time.Hour)

	f.send(outageEvent("ep-5", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-5")

	f.elapse(t, 2*time.Hour)
	f.send(outageEvent("ep-5", alert.SeverityCritical, false))
	f.barrier(t)

	rows := f.rows(t, "test", "ep-5")
	require.Len(t, rows, 1, "the event must promote the held row, not add one")
	assert.Equal(t, held.ID, rows[0].ID)
	f.assertPromotedOnce(t, held.ID)
}

func TestSilenced_ResolveByEntityDropsTheHeldRow(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-8")
	f.start(fastRecheck)

	f.send(outageEvent("ep-8", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-8")

	f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-8")
	assert.Equal(t, alert.StatusResolved, f.status(t, held.ID))

	f.elapse(t, 2*time.Hour)
	f.assertNeverNotified(t, held.ID)
}

func TestSilenced_RestartReloadsRecentHeldRows(t *testing.T) {
	f := newSilenceFixture(t)
	f.silence(t, alert.SilenceRule{Source: "test"})

	longAgo := time.Now().Add(-48 * time.Hour)
	old := &alert.Alert{
		Source: "test", AlertType: "down", Severity: alert.SeverityCritical, Status: alert.StatusSilenced,
		Message: "ep-old down", EntityType: "endpoint", EntityID: "ep-old", EntityName: "ep-old",
		Details: "{}", FiredAt: longAgo, CreatedAt: longAgo,
	}
	_, err := f.alerts.InsertAlert(f.ctx, old)
	require.NoError(t, err)

	f.start(fastRecheck)
	f.send(outageEvent("ep-6", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-6")

	f.start(fastRecheck)
	f.elapse(t, 2*time.Hour)

	f.assertPromotedOnce(t, held.ID)
	assert.Equal(t, alert.StatusSilenced, f.status(t, old.ID), "a held row older than the reload bound must stay as it is")
	assert.Zero(t, f.deliveries(t, old.ID))
}

func TestSilenced_RestartAfterTheWindowDoesNotPromote(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "ep-7")
	f.start(fastRecheck)

	f.send(outageEvent("ep-7", alert.SeverityCritical, false))
	f.barrier(t)
	held := f.held(t, "ep-7")

	f.halt()
	f.elapse(t, 2*time.Hour)
	f.start(fastRecheck)
	f.barrier(t)

	f.assertNeverNotified(t, held.ID)
	assert.Equal(t, alert.StatusSilenced, f.status(t, held.ID))
}

func TestSilenced_ContainerBackDuringTheWindowStaysQuiet(t *testing.T) {
	f := newSilenceFixture(t)
	containers := store.NewContainerStore(f.db)
	stopped := &container.Container{
		ExternalID: "api-external", Name: "api", Image: "busybox:latest", State: container.StateExited,
		AlertSeverity: container.SeverityCritical, RuntimeType: "docker",
		FirstSeenAt: time.Now().Add(-24 * time.Hour), LastStateChangeAt: time.Now().Add(-10 * time.Minute),
	}
	id, err := containers.InsertContainer(f.ctx, stopped)
	require.NoError(t, err)
	f.silence(t, alert.SilenceRule{Source: alert.SourceContainer, EntityType: "container", EntityID: &id})
	f.start(fastRecheck)

	detector := alert.NewDownDetector(containers, f.alerts, 5*time.Minute, f.logger)
	detector.SetEmitter(f.send)
	detector.SetSilenced(f.engine.SilencedAlerts)

	require.NoError(t, detector.Check(f.ctx))
	f.barrier(t)
	rows := f.rows(t, alert.SourceContainer, id)
	require.Len(t, rows, 1)
	require.Equal(t, alert.StatusSilenced, rows[0].Status)

	stopped.State = container.StateRunning
	stopped.LastStateChangeAt = time.Now()
	_, err = containers.InsertContainer(f.ctx, stopped)
	require.NoError(t, err)
	require.NoError(t, detector.Check(f.ctx))
	f.barrier(t)

	f.elapse(t, 2*time.Hour)
	require.NoError(t, detector.Check(f.ctx))
	f.barrier(t)
	f.assertNeverNotified(t, rows[0].ID)
	assert.Equal(t, alert.StatusResolved, f.status(t, rows[0].ID), "the container came back during the window")
}
