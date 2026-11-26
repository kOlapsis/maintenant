// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

var errInjected = errors.New("injected storage failure")

func takeFailure(n *atomic.Int32) bool {
	for {
		v := n.Load()
		if v <= 0 {
			return false
		}
		if n.CompareAndSwap(v, v-1) {
			return true
		}
	}
}

type flakyEscStore struct {
	esc.Store
	esc.DeliveryQueue
	insertRunFails  atomic.Int32
	reserveFails    atomic.Int32
	refusedPolicyID atomic.Pointer[string]
}

func (s *flakyEscStore) InsertRun(ctx context.Context, r *esc.Run) (string, error) {
	if refused := s.refusedPolicyID.Load(); refused != nil && r.PolicyID != nil && *r.PolicyID == *refused {
		return "", errInjected
	}
	if takeFailure(&s.insertRunFails) {
		return "", errInjected
	}
	return s.Store.InsertRun(ctx, r)
}

func (s *flakyEscStore) InsertDelivery(ctx context.Context, d *esc.Delivery) (string, error) {
	if takeFailure(&s.reserveFails) {
		return "", errInjected
	}
	return s.Store.InsertDelivery(ctx, d)
}

func (s *flakyEscStore) ReserveDelivery(ctx context.Context, d *esc.Delivery) (bool, error) {
	if takeFailure(&s.reserveFails) {
		return false, errInjected
	}
	return s.Store.ReserveDelivery(ctx, d)
}

type flakyChannels struct {
	alert.ChannelStore
	mu       sync.Mutex
	getFails map[string]int
}

func (c *flakyChannels) failGet(id string, times int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getFails[id] += times
}

func (c *flakyChannels) GetChannel(ctx context.Context, id string) (*alert.NotificationChannel, error) {
	c.mu.Lock()
	fail := c.getFails[id] > 0
	if fail {
		c.getFails[id]--
	}
	c.mu.Unlock()
	if fail {
		return nil, errInjected
	}
	return c.ChannelStore.GetChannel(ctx, id)
}

type hookedAlerts struct {
	*store.AlertStoreImpl
	mu              sync.Mutex
	afterGet        func()
	beforeEscalated func()
}

func (s *hookedAlerts) setHook(hook *func(), fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*hook = fn
}

func (s *hookedAlerts) takeHook(hook *func()) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn := *hook
	*hook = nil
	return fn
}

func (s *hookedAlerts) GetAlert(ctx context.Context, id string) (*alert.Alert, error) {
	a, err := s.AlertStoreImpl.GetAlert(ctx, id)
	if fn := s.takeHook(&s.afterGet); fn != nil {
		fn()
	}
	return a, err
}

func (s *hookedAlerts) SetEscalatedAt(ctx context.Context, id string, at time.Time) error {
	if fn := s.takeHook(&s.beforeEscalated); fn != nil {
		fn()
	}
	return s.AlertStoreImpl.SetEscalatedAt(ctx, id, at)
}

type dbHarness struct {
	t        *testing.T
	ctx      context.Context
	store    *flakyEscStore
	alerts   *hookedAlerts
	channels *flakyChannels
	supp     *fakeSuppressor
	runner   *Runner

	clockMu sync.Mutex
	now     time.Time
}

func newDBHarness(t *testing.T, sender Sender) *dbHarness {
	t.Helper()
	setEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	escStore := store.NewEscalationStore(db)
	h := &dbHarness{
		t:        t,
		ctx:      context.Background(),
		store:    &flakyEscStore{Store: escStore, DeliveryQueue: escStore},
		alerts:   &hookedAlerts{AlertStoreImpl: store.NewAlertStore(db)},
		channels: &flakyChannels{ChannelStore: store.NewChannelStore(db), getFails: map[string]int{}},
		supp:     &fakeSuppressor{},
		now:      time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}
	h.restart(sender)
	return h
}

// restart replaces the runner by a new one over the same database, as a process start does.
func (h *dbHarness) restart(sender Sender) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h.runner = NewRunner(RunnerDeps{
		Store:         h.store,
		AlertStore:    h.alerts,
		ChannelStore:  h.channels,
		Notifier:      sender,
		Suppressor:    h.supp,
		Service:       NewService(h.store, h.channels, nil, h.supp, logger),
		Logger:        logger,
		Clock:         h.clock,
		OrphanTimeout: time.Hour,
	})
}

func (h *dbHarness) clock() time.Time {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	return h.now
}

func (h *dbHarness) advance(d time.Duration) {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	h.now = h.now.Add(d)
}

func (h *dbHarness) addChannel(name, url string, enabled bool) string {
	h.t.Helper()
	id, err := h.channels.InsertChannel(h.ctx, &alert.NotificationChannel{
		Name: name, Type: "webhook", URL: url, Enabled: enabled,
	})
	require.NoError(h.t, err)
	return id
}

func (h *dbHarness) addAlert() *alert.Alert {
	h.t.Helper()
	a := criticalAlert("")
	a.FiredAt = h.clock()
	_, err := h.alerts.InsertAlert(h.ctx, a)
	require.NoError(h.t, err)
	return a
}

func (h *dbHarness) addPolicy(levels ...esc.Level) string {
	h.t.Helper()
	id, err := h.store.InsertPolicy(h.ctx, &esc.Policy{
		Name:    "policy-" + uid.New(),
		Active:  true,
		Filters: esc.Filters{Severities: []string{alert.SeverityCritical}, Scopes: []esc.Scope{}},
		Levels:  levels,
	})
	require.NoError(h.t, err)
	return id
}

func (h *dbHarness) runs(a *alert.Alert) []*esc.Run {
	h.t.Helper()
	runs, err := h.store.SelectRunsByAlert(h.ctx, a.ID)
	require.NoError(h.t, err)
	return runs
}

func (h *dbHarness) onlyRun(a *alert.Alert) *esc.Run {
	h.t.Helper()
	runs := h.runs(a)
	require.Len(h.t, runs, 1)
	return runs[0]
}

func (h *dbHarness) deliveries(runID string) []*esc.Delivery {
	h.t.Helper()
	ds, err := h.store.SelectRunDeliveries(h.ctx, runID)
	require.NoError(h.t, err)
	return ds
}

func (h *dbHarness) cycle() {
	h.t.Helper()
	require.NoError(h.t, h.runner.EvaluateCycle(h.ctx))
}

// settle returns once every send the runner started has recorded its outcome.
func (h *dbHarness) settle() { h.runner.sending.Wait() }

func sentTo(calls []senderCall, channelID string) int {
	n := 0
	for _, c := range calls {
		if c.ChannelID == channelID {
			n++
		}
	}
	return n
}
