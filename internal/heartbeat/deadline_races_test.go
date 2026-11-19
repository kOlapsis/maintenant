// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

type hookedStore struct {
	heartbeat.HeartbeatStore
	afterList  func()
	afterToken func()
}

func (s *hookedStore) ListOverdueHeartbeats(ctx context.Context, now time.Time) ([]*heartbeat.Heartbeat, error) {
	hbs, err := s.HeartbeatStore.ListOverdueHeartbeats(ctx, now)
	if f := s.afterList; f != nil {
		s.afterList = nil
		f()
	}
	return hbs, err
}

func (s *hookedStore) GetHeartbeatByPingToken(ctx context.Context, token string) (*heartbeat.Heartbeat, error) {
	h, err := s.HeartbeatStore.GetHeartbeatByPingToken(ctx, token)
	if f := s.afterToken; f != nil {
		s.afterToken = nil
		f()
	}
	return h, err
}

type raceBench struct {
	real   *store.HeartbeatStore
	hooked *hookedStore
	svc    *heartbeat.Service

	mu     sync.Mutex
	alerts []string
}

func newRaceBench(t *testing.T) *raceBench {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := &raceBench{real: store.NewHeartbeatStore(storetest.Open(t, logger))}
	b.hooked = &hookedStore{HeartbeatStore: b.real}
	b.svc = heartbeat.NewService(heartbeat.Deps{
		Store:  b.hooked,
		Logger: logger,
		AlertCallback: func(_ *heartbeat.Heartbeat, kind string, _ map[string]interface{}) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.alerts = append(b.alerts, kind)
		},
	})
	return b
}

func (b *raceBench) raised() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.alerts...)
}

func (b *raceBench) seed(t *testing.T, status heartbeat.HeartbeatStatus, deadline time.Time) *heartbeat.Heartbeat {
	t.Helper()
	h := &heartbeat.Heartbeat{ID: uid.New(), PingToken: uid.New(), Name: "nightly backup", IntervalSeconds: 3600, GraceSeconds: 600}
	_, err := b.real.CreateHeartbeat(context.Background(), h)
	require.NoError(t, err)
	lastPing := deadline.Add(-time.Hour)
	require.NoError(t, b.real.UpdateHeartbeatState(context.Background(), h.ID, status, heartbeat.AlertNormal,
		&lastPing, &deadline, nil, nil, nil, 0, 3))
	return h
}

func (b *raceBench) overdueAt(t *testing.T, at time.Time) map[string]bool {
	t.Helper()
	overdue, err := b.real.ListOverdueHeartbeats(context.Background(), at)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, h := range overdue {
		ids[h.ID] = true
	}
	return ids
}

func TestStartup_PostponesDeadlinesThatPassedWhileStopped(t *testing.T) {
	ctx := context.Background()
	b := newRaceBench(t)
	stale := b.seed(t, heartbeat.StatusUp, time.Now().Add(-2*time.Hour))
	started := b.seed(t, heartbeat.StatusStarted, time.Now().Add(-30*time.Minute))
	future := time.Unix(time.Now().Add(20*time.Minute).Unix(), 0)
	notDue := b.seed(t, heartbeat.StatusUp, future)
	down := b.seed(t, heartbeat.StatusDown, time.Now().Add(-time.Hour))

	runCtx, cancel := context.WithCancel(ctx)
	b.svc.StartDeadlineChecker(runCtx)
	cancel()
	startedAt := time.Now()

	heartbeat.RunDeadlineCheck(ctx, b.svc)

	assert.Empty(t, b.raised(), "a deadline that fell while the server was stopped raises nothing")
	for _, h := range []*heartbeat.Heartbeat{stale, started} {
		got, err := b.svc.GetHeartbeat(ctx, h.ID)
		require.NoError(t, err)
		assert.NotEqual(t, heartbeat.StatusDown, got.Status)
		require.NotNil(t, got.NextDeadlineAt)
		assert.WithinDuration(t, startedAt.Add(4200*time.Second), *got.NextDeadlineAt, 10*time.Second, "now + interval + grace")
		assert.False(t, b.overdueAt(t, got.NextDeadlineAt.Add(-time.Second))[h.ID], "not overdue before the new deadline")
		assert.True(t, b.overdueAt(t, got.NextDeadlineAt.Add(time.Second))[h.ID], "overdue right after the new deadline")
	}

	got, err := b.svc.GetHeartbeat(ctx, notDue.ID)
	require.NoError(t, err)
	assert.Equal(t, future.Unix(), got.NextDeadlineAt.Unix(), "a deadline still ahead is left alone")
	got, err = b.svc.GetHeartbeat(ctx, down.ID)
	require.NoError(t, err)
	assert.Equal(t, heartbeat.StatusDown, got.Status)
}

func TestDeadlineCheck_DoesNotOverwriteAPingThatLandedAfterItsRead(t *testing.T) {
	ctx := context.Background()
	b := newRaceBench(t)
	h := b.seed(t, heartbeat.StatusUp, time.Now().Add(-time.Minute))
	b.hooked.afterList = func() {
		_, err := b.svc.ProcessPing(ctx, h.PingToken, "10.0.0.1", "GET", nil, nil)
		require.NoError(t, err)
	}

	heartbeat.RunDeadlineCheck(ctx, b.svc)

	got, err := b.svc.GetHeartbeat(ctx, h.ID)
	require.NoError(t, err)
	assert.Equal(t, heartbeat.StatusUp, got.Status, "the ping wins, the checker does not put it down")
	assert.Equal(t, heartbeat.AlertNormal, got.AlertState)
	require.NotNil(t, got.NextDeadlineAt)
	assert.True(t, got.NextDeadlineAt.After(time.Now()), "the deadline is the one the ping set")
	assert.Empty(t, b.raised(), "no false alert")
}

func TestPing_RecoversWhenTheDeadlineCheckFiredAfterItsRead(t *testing.T) {
	cases := []struct {
		name string
		ping func(ctx context.Context, b *raceBench, h *heartbeat.Heartbeat) error
	}{
		{"success ping", func(ctx context.Context, b *raceBench, h *heartbeat.Heartbeat) error {
			_, err := b.svc.ProcessPing(ctx, h.PingToken, "10.0.0.1", "GET", nil, nil)
			return err
		}},
		{"exit code 0 ping", func(ctx context.Context, b *raceBench, h *heartbeat.Heartbeat) error {
			_, err := b.svc.ProcessExitCodePing(ctx, h.PingToken, 0, "10.0.0.1", "GET", nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := newRaceBench(t)
			h := b.seed(t, heartbeat.StatusUp, time.Now().Add(-time.Minute))
			b.hooked.afterToken = func() { heartbeat.RunDeadlineCheck(ctx, b.svc) }

			require.NoError(t, tc.ping(ctx, b, h))

			assert.Equal(t, []string{"alert", "recovery"}, b.raised(), "the alert the checker raised is recovered")
			got, err := b.svc.GetHeartbeat(ctx, h.ID)
			require.NoError(t, err)
			assert.Equal(t, heartbeat.StatusUp, got.Status)
			assert.Equal(t, heartbeat.AlertNormal, got.AlertState)
		})
	}
}
