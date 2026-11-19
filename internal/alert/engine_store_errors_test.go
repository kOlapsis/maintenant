// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

var errStoreDown = errors.New("store unavailable")

const sentinelSource = "sentinel"

// failures makes the named store methods fail a set number of times, or always for -1.
type failures struct {
	mu    sync.Mutex
	left  map[string]int
	calls map[string]int
}

func (f *failures) set(method string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.left[method] = n
}

func (f *failures) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *failures) hit(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method]++
	switch n := f.left[method]; {
	case n == 0:
		return nil
	case n > 0:
		f.left[method] = n - 1
	}
	return errStoreDown
}

type flakyAlertStore struct {
	alert.AlertStore
	*failures
}

func (s flakyAlertStore) InsertAlert(ctx context.Context, a *alert.Alert) (string, error) {
	if a.Source != sentinelSource {
		if err := s.hit("InsertAlert"); err != nil {
			return "", err
		}
	}
	return s.AlertStore.InsertAlert(ctx, a)
}

func (s flakyAlertStore) UpdateAlertStatus(ctx context.Context, id, status string, resolvedAt *time.Time, resolvedByID *string) error {
	if err := s.hit("UpdateAlertStatus"); err != nil {
		return err
	}
	return s.AlertStore.UpdateAlertStatus(ctx, id, status, resolvedAt, resolvedByID)
}

func (s flakyAlertStore) UpdateAlertOnEscalation(ctx context.Context, id, severity, message, entityName, details string) error {
	if err := s.hit("UpdateAlertOnEscalation"); err != nil {
		return err
	}
	return s.AlertStore.UpdateAlertOnEscalation(ctx, id, severity, message, entityName, details)
}

func (s flakyAlertStore) ListActiveAlerts(ctx context.Context) ([]*alert.Alert, error) {
	if err := s.hit("ListActiveAlerts"); err != nil {
		return nil, err
	}
	return s.AlertStore.ListActiveAlerts(ctx)
}

type flakyTriggerStore struct {
	alert.TriggerStore
	*failures
}

func (s flakyTriggerStore) ListEnabledTriggers(ctx context.Context) ([]*alert.AlertTrigger, error) {
	if err := s.hit("ListEnabledTriggers"); err != nil {
		return nil, err
	}
	return s.TriggerStore.ListEnabledTriggers(ctx)
}

type flakyChannelStore struct {
	alert.ChannelStore
	*failures
}

func (s flakyChannelStore) GetChannel(ctx context.Context, id string) (*alert.NotificationChannel, error) {
	if err := s.hit("GetChannel"); err != nil {
		return nil, err
	}
	return s.ChannelStore.GetChannel(ctx, id)
}

type resolveRecorder struct {
	mu       sync.Mutex
	resolved []string
}

func (r *resolveRecorder) EvaluateCycle(context.Context) error                { return nil }
func (r *resolveRecorder) OnAlertCreated(context.Context, *alert.Alert) error { return nil }
func (r *resolveRecorder) OnAlertAcknowledged(context.Context, string, alert.Acknowledgment) error {
	return nil
}
func (r *resolveRecorder) OnEditionDowngraded(context.Context) error { return nil }

func (r *resolveRecorder) OnAlertResolved(_ context.Context, id string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved = append(r.resolved, id)
	return nil
}

func (r *resolveRecorder) resolvedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.resolved...)
}

// storeErrorFixture is an engine over a real database whose stores fail on demand.
// Its single trigger notifies every alert of source "test", recoveries included.
type storeErrorFixture struct {
	ctx       context.Context
	fail      *failures
	alerts    alert.AlertStore
	channels  alert.ChannelStore
	escalator *resolveRecorder
	logs      *syncBuffer
	engine    *alert.Engine
	sentinels int
}

func newStoreErrorFixture(t *testing.T) *storeErrorFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	db := storetest.Open(t, logger)

	f := &failures{left: map[string]int{}, calls: map[string]int{}}
	alerts := store.NewAlertStore(db)
	channels := store.NewChannelStore(db)
	triggers := store.NewTriggerStore(db)

	chID, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "hook", Type: "webhook", URL: "https://example.com/hook", Enabled: true})
	require.NoError(t, err)
	_, err = triggers.InsertTrigger(ctx, &alert.AlertTrigger{
		Name: "all", FilterSources: "test", Enabled: true, NotifyOnResolve: true, ChannelIDs: []string{chID},
	})
	require.NoError(t, err)

	// Never started: the delivery row is the notification, and no send outlives the test.
	notifier := alert.NewNotifier(channels, logger, true)

	escalator := &resolveRecorder{}
	eng := alert.NewEngine(alert.EngineDeps{
		AlertStore:   flakyAlertStore{alerts, f},
		ChannelStore: flakyChannelStore{channels, f},
		TriggerStore: flakyTriggerStore{triggers, f},
		SilenceStore: store.NewSilenceStore(db),
		Notifier:     notifier,
		Logger:       logger,
	})
	eng.SetEscalator(escalator)

	return &storeErrorFixture{
		ctx: ctx, fail: f, alerts: alerts, channels: channels,
		escalator: escalator, logs: logs, engine: eng,
	}
}

func outageEvent(entityID, severity string, recover bool) alert.Event {
	return alert.Event{
		Source: "test", AlertType: "down", Severity: severity, IsRecover: recover,
		Message:    fmt.Sprintf("%s down=%t", entityID, !recover),
		EntityType: "endpoint", EntityID: entityID, EntityName: entityID,
		Timestamp: time.Now(),
	}
}

func (f *storeErrorFixture) send(evt alert.Event) { f.engine.EventChannel() <- evt }

// barrier returns once the engine has fully processed every event sent before it.
func (f *storeErrorFixture) barrier(t *testing.T) {
	t.Helper()
	id := fmt.Sprintf("sentinel-%d", f.sentinels)
	f.sentinels++
	f.send(alert.Event{
		Source: sentinelSource, AlertType: "barrier", Severity: alert.SeverityInfo,
		EntityType: "sentinel", EntityID: id, EntityName: id, Timestamp: time.Now(),
	})
	require.Eventually(t, func() bool {
		return len(f.activeRows(id)) == 1
	}, 10*time.Second, 5*time.Millisecond, "the engine must get past the events sent before the barrier")
}

func (f *storeErrorFixture) activeRows(entityID string) []*alert.Alert {
	active, err := f.alerts.ListActiveAlerts(f.ctx)
	if err != nil {
		return nil
	}
	var out []*alert.Alert
	for _, a := range active {
		if a.EntityID == entityID {
			out = append(out, a)
		}
	}
	return out
}

func (f *storeErrorFixture) waitActive(t *testing.T, entityID string) *alert.Alert {
	t.Helper()
	var got *alert.Alert
	require.Eventually(t, func() bool {
		rows := f.activeRows(entityID)
		if len(rows) != 1 {
			return false
		}
		got = rows[0]
		return true
	}, 10*time.Second, 5*time.Millisecond, "the alert must be active in the database")
	return got
}

func (f *storeErrorFixture) get(t *testing.T, id string) *alert.Alert {
	t.Helper()
	a, err := f.alerts.GetAlert(f.ctx, id)
	require.NoError(t, err)
	require.NotNil(t, a)
	return a
}

func (f *storeErrorFixture) deliveries(t *testing.T, alertID string) int {
	t.Helper()
	ds, err := f.channels.ListDeliveriesByAlert(f.ctx, alertID)
	require.NoError(t, err)
	return len(ds)
}

// inMemory is the engine's active alert count, sentinels left out.
func (f *storeErrorFixture) inMemory() int { return f.engine.AlertCount() - f.sentinels }

func TestEngineStoreErrors_InsertFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.fail.set("InsertAlert", 1)
	f.send(outageEvent("ep-1", alert.SeverityCritical, false))
	f.barrier(t)

	rows := f.activeRows("ep-1")
	require.Len(t, rows, 1, "the alert must exist in the database after a failed first insert")
	assert.Equal(t, 1, f.inMemory(), "the alert must be active in memory")
	assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "the alert must be notified once")
}

func TestEngineStoreErrors_RecoveryInsertFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.send(outageEvent("ep-2", alert.SeverityCritical, false))
	original := f.waitActive(t, "ep-2")

	f.fail.set("InsertAlert", 1)
	f.send(outageEvent("ep-2", alert.SeverityInfo, true))
	f.barrier(t)

	got := f.get(t, original.ID)
	assert.Equal(t, alert.StatusResolved, got.Status, "the alert must end resolved in the database")
	assert.NotNil(t, got.ResolvedByID, "the alert must point at its recovery")
	assert.Equal(t, 0, f.inMemory(), "the alert must leave the memory")
	assert.Equal(t, []string{original.ID}, f.escalator.resolvedIDs(), "OnAlertResolved must run once")
	assert.Equal(t, 2, f.deliveries(t, original.ID), "the fire and the recovery must each be notified once")
}

func TestEngineStoreErrors_RecoveryStatusUpdateFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.send(outageEvent("ep-3", alert.SeverityCritical, false))
	original := f.waitActive(t, "ep-3")

	f.fail.set("UpdateAlertStatus", 1)
	f.send(outageEvent("ep-3", alert.SeverityInfo, true))
	f.barrier(t)

	assert.Empty(t, f.activeRows("ep-3"), "no active row may be left behind for the next start to reload")
	assert.Equal(t, alert.StatusResolved, f.get(t, original.ID).Status)
	assert.Equal(t, 0, f.inMemory())
	assert.Equal(t, []string{original.ID}, f.escalator.resolvedIDs(), "OnAlertResolved must run once")
}

func TestEngineStoreErrors_ResolveByEntityWriteFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.send(outageEvent("ep-4", alert.SeverityCritical, false))
	original := f.waitActive(t, "ep-4")

	f.fail.set("UpdateAlertStatus", 1)
	f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-4")

	assert.Equal(t, alert.StatusResolved, f.get(t, original.ID).Status, "the alert must end resolved in the database")
	assert.Equal(t, 0, f.inMemory(), "the alert must leave the memory")
	assert.Equal(t, []string{original.ID}, f.escalator.resolvedIDs(), "OnAlertResolved must run once")
}

func TestEngineStoreErrors_SeverityRaiseWriteFailsOnce(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.send(outageEvent("ep-5", alert.SeverityWarning, false))
	original := f.waitActive(t, "ep-5")

	f.fail.set("UpdateAlertOnEscalation", 1)
	f.send(outageEvent("ep-5", alert.SeverityCritical, false))
	f.barrier(t)

	assert.Equal(t, alert.SeverityCritical, f.get(t, original.ID).Severity, "the raise must reach the database")
	assert.Equal(t, 2, f.deliveries(t, original.ID), "the fire and the raise must each be notified once")
}

func TestEngineStoreErrors_WriteAlwaysFails(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-6", alert.SeverityCritical, false))
		f.barrier(t)

		rows, err := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: "test"})
		require.NoError(t, err)
		assert.Empty(t, rows, "nothing may reach the database")
		assert.Equal(t, 0, f.inMemory(), "the memory must match the database")
		assert.Greater(t, f.fail.count("InsertAlert"), 1, "the insert must be retried")
		assert.Contains(t, f.logs.String(), errStoreDown.Error(), "the error must be logged")
	})

	t.Run("recovery", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.send(outageEvent("ep-7", alert.SeverityCritical, false))
		original := f.waitActive(t, "ep-7")

		f.fail.set("UpdateAlertStatus", -1)
		f.send(outageEvent("ep-7", alert.SeverityInfo, true))
		f.barrier(t)

		assert.Equal(t, alert.StatusActive, f.get(t, original.ID).Status)
		assert.Equal(t, 1, f.inMemory(), "the memory must still hold the alert the database holds")
		assert.Empty(t, f.escalator.resolvedIDs(), "an alert still active must not be reported resolved")
		assert.Contains(t, f.logs.String(), errStoreDown.Error(), "the error must be logged")

		f.fail.set("UpdateAlertStatus", 0)
		f.send(outageEvent("ep-7", alert.SeverityInfo, true))
		f.barrier(t)
		assert.Equal(t, alert.StatusResolved, f.get(t, original.ID).Status, "a later recovery must still find the alert")
	})

	t.Run("severity raise", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.send(outageEvent("ep-8", alert.SeverityWarning, false))
		original := f.waitActive(t, "ep-8")

		f.fail.set("UpdateAlertOnEscalation", -1)
		f.send(outageEvent("ep-8", alert.SeverityCritical, false))
		f.barrier(t)

		assert.Equal(t, alert.SeverityWarning, f.get(t, original.ID).Severity)
		assert.Equal(t, 1, f.deliveries(t, original.ID), "a raise that is not stored must not be notified")
		assert.Contains(t, f.logs.String(), errStoreDown.Error(), "the error must be logged")

		f.fail.set("UpdateAlertOnEscalation", 0)
		f.send(outageEvent("ep-8", alert.SeverityCritical, false))
		f.barrier(t)
		assert.Equal(t, alert.SeverityCritical, f.get(t, original.ID).Severity, "the memory must not claim a severity the database lacks")
		assert.Equal(t, 2, f.deliveries(t, original.ID))
	})

	t.Run("resolve by entity", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.send(outageEvent("ep-9", alert.SeverityCritical, false))
		original := f.waitActive(t, "ep-9")

		f.fail.set("UpdateAlertStatus", -1)
		f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-9")

		assert.Equal(t, alert.StatusActive, f.get(t, original.ID).Status)
		assert.Equal(t, 1, f.inMemory(), "the memory must still hold the alert the database holds")
		assert.Empty(t, f.escalator.resolvedIDs())
		assert.Contains(t, f.logs.String(), errStoreDown.Error(), "the error must be logged")
	})
}

func TestEngineStoreErrors_ReloadFailsOnceAtStart(t *testing.T) {
	f := newStoreErrorFixture(t)
	original := &alert.Alert{
		Source: "test", AlertType: "down", Severity: alert.SeverityCritical, Status: alert.StatusActive,
		EntityType: "endpoint", EntityID: "ep-10", EntityName: "ep-10", FiredAt: time.Now(),
	}
	_, err := f.alerts.InsertAlert(f.ctx, original)
	require.NoError(t, err)

	f.fail.set("ListActiveAlerts", 1)
	f.engine.Start(f.ctx)
	assert.Equal(t, 1, f.engine.AlertCount(), "the active alert must be reloaded")

	f.send(outageEvent("ep-10", alert.SeverityInfo, true))
	f.barrier(t)
	assert.Equal(t, alert.StatusResolved, f.get(t, original.ID).Status, "a recovery must resolve the reloaded alert")
}
