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

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 048-heartbeat-edit: the deadline checker on an edited heartbeat, with the real store
// (SQLite, or PostgreSQL through MAINTENANT_TEST_DATABASE_URL).

type raisedAlert struct {
	HeartbeatID, Kind, AlertType string
}

type checkBench struct {
	svc *heartbeat.Service
	hs  *store.HeartbeatStore

	mu          sync.Mutex
	alerts      []raisedAlert
	alertEvents []raisedAlert
}

func newCheckBench(t *testing.T) *checkBench {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := &checkBench{hs: store.NewHeartbeatStore(storetest.Open(t, logger))}
	b.svc = heartbeat.NewService(heartbeat.Deps{
		Store:  b.hs,
		Logger: logger,
		EventCallback: func(eventType string, data interface{}) {
			if eventType != event.HeartbeatAlert {
				return
			}
			m, _ := data.(map[string]interface{})
			id, _ := m["heartbeat_id"].(string)
			alertType, _ := m["alert_type"].(string)
			b.mu.Lock()
			defer b.mu.Unlock()
			b.alertEvents = append(b.alertEvents, raisedAlert{id, eventType, alertType})
		},
		AlertCallback: func(h *heartbeat.Heartbeat, kind string, details map[string]interface{}) {
			alertType, _ := details["alert_type"].(string)
			b.mu.Lock()
			defer b.mu.Unlock()
			b.alerts = append(b.alerts, raisedAlert{h.ID, kind, alertType})
		},
	})
	return b
}

func (b *checkBench) raised() ([]raisedAlert, []raisedAlert) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]raisedAlert(nil), b.alerts...), append([]raisedAlert(nil), b.alertEvents...)
}

func (b *checkBench) seed(t *testing.T, name string, interval, grace int) *heartbeat.Heartbeat {
	t.Helper()
	h := &heartbeat.Heartbeat{ID: uid.New(), PingToken: uid.New(), Name: name, IntervalSeconds: interval, GraceSeconds: grace}
	_, err := b.hs.CreateHeartbeat(context.Background(), h)
	require.NoError(t, err)
	return h
}

func (b *checkBench) arrange(t *testing.T, h *heartbeat.Heartbeat, status heartbeat.HeartbeatStatus, lastPing time.Time, runStart *time.Time) time.Time {
	t.Helper()
	start := lastPing
	if runStart != nil {
		start = *runStart
	}
	deadline := start.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	require.NoError(t, b.hs.UpdateHeartbeatState(context.Background(), h.ID, status, heartbeat.AlertNormal,
		&lastPing, &deadline, runStart, nil, nil, 0, 3))
	return start
}

func (b *checkBench) edit(t *testing.T, h *heartbeat.Heartbeat, interval, grace int) *heartbeat.Heartbeat {
	t.Helper()
	name := h.Name
	got, err := b.svc.UpdateHeartbeat(context.Background(), h.ID,
		heartbeat.UpdateHeartbeatInput{Name: &name, IntervalSeconds: &interval, GraceSeconds: &grace})
	require.NoError(t, err)
	require.NotNil(t, got)
	return got
}

func (b *checkBench) overdueAt(t *testing.T, at time.Time) map[string]bool {
	t.Helper()
	overdue, err := b.hs.ListOverdueHeartbeats(context.Background(), at)
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, h := range overdue {
		ids[h.ID] = true
	}
	return ids
}

func secondsAgo(d time.Duration) time.Time { return time.Unix(time.Now().Add(-d).Unix(), 0) }

// US2 scenario 4, FR-006, FR-005, US2 independent test: a recomputed deadline already passed is handled at the next
// check like any missed deadline (down, deadline_missed alert), the edit alone raises nothing, and once pinged again the
// deadline counts from that ping with the values in force.
func TestEditedDeadlineAlreadyPassed_GoesDownAtTheNextCheckWithTheUsualAlert(t *testing.T) {
	cases := []struct {
		name                  string
		interval, grace       int
		status                heartbeat.HeartbeatStatus
		lastPing              time.Duration
		runStart              time.Duration
		newInterval, newGrace int
	}{
		{"up, pinged 3h ago, one day shortened to one hour", 86400, 600, heartbeat.StatusUp, 3 * time.Hour, 0, 3600, 60},
		{"started 2h ago, one day shortened to 30 min", 86400, 600, heartbeat.StatusStarted, 30 * time.Hour, 2 * time.Hour, 1800, 300},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := newCheckBench(t)
			h := b.seed(t, "nightly backup", tc.interval, tc.grace)
			var runStart *time.Time
			if tc.runStart > 0 {
				s := secondsAgo(tc.runStart)
				runStart = &s
			}
			start := b.arrange(t, h, tc.status, secondsAgo(tc.lastPing), runStart)

			edited := b.edit(t, h, tc.newInterval, tc.newGrace)
			require.NotNil(t, edited.NextDeadlineAt)
			assert.Equal(t, start.Unix()+int64(tc.newInterval+tc.newGrace), edited.NextDeadlineAt.Unix())
			assert.True(t, edited.NextDeadlineAt.Before(time.Now()), "the recomputed deadline is already passed")
			assert.Equal(t, tc.status, edited.Status, "the edit alone does not put the heartbeat down")
			alerts, alertEvents := b.raised()
			assert.Empty(t, alerts, "the edit alone raises no alert")
			assert.Empty(t, alertEvents)

			heartbeat.RunDeadlineCheck(ctx, b.svc)

			got, err := b.svc.GetHeartbeat(ctx, h.ID)
			require.NoError(t, err)
			assert.Equal(t, heartbeat.StatusDown, got.Status, "the next check puts it down")
			assert.Equal(t, heartbeat.AlertAlerting, got.AlertState)
			alerts, alertEvents = b.raised()
			assert.Equal(t, []raisedAlert{{h.ID, "alert", heartbeat.AlertTypeDeadlineMissed}}, alerts, "the usual missed-deadline alert goes out")
			assert.Equal(t, []raisedAlert{{h.ID, event.HeartbeatAlert, heartbeat.AlertTypeDeadlineMissed}}, alertEvents)

			_, err = b.svc.ProcessPing(ctx, h.PingToken, "10.0.0.1", "GET", nil, nil)
			require.NoError(t, err)
			back := b.edit(t, got, tc.interval, tc.grace)
			pinged, err := b.svc.GetHeartbeat(ctx, h.ID)
			require.NoError(t, err)
			require.NotNil(t, pinged.LastPingAt)
			require.NotNil(t, back.NextDeadlineAt)
			assert.Equal(t, heartbeat.StatusUp, back.Status)
			assert.Equal(t, pinged.LastPingAt.Unix()+int64(tc.interval+tc.grace), back.NextDeadlineAt.Unix(),
				"set back to the old values after a ping: last ping + interval + grace")
		})
	}
}

// US2 "why", FR-004: a lengthened interval no longer counts as missed at the old deadline, only at the new one.
func TestLengthenedInterval_NoLongerOverdueAtTheOldDeadline(t *testing.T) {
	cases := []struct {
		name                  string
		interval, grace       int
		status                heartbeat.HeartbeatStatus
		lastPing, runStart    time.Duration
		newInterval, newGrace int
	}{
		{"up, pinged 50 min ago, one hour lengthened to one day", 3600, 60, heartbeat.StatusUp, 50 * time.Minute, 0, 86400, 600},
		{"started 20 min ago, 30 min lengthened to 2h", 1800, 60, heartbeat.StatusStarted, 3 * time.Hour, 20 * time.Minute, 7200, 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := newCheckBench(t)
			h := b.seed(t, "hourly sync", tc.interval, tc.grace)
			var runStart *time.Time
			if tc.runStart > 0 {
				s := secondsAgo(tc.runStart)
				runStart = &s
			}
			start := b.arrange(t, h, tc.status, secondsAgo(tc.lastPing), runStart)
			inAnHour := time.Now().Add(time.Hour)
			require.True(t, b.overdueAt(t, inAnHour)[h.ID], "fixture: with the old values it is overdue within the hour")

			b.edit(t, h, tc.newInterval, tc.newGrace)

			assert.False(t, b.overdueAt(t, inAnHour)[h.ID], "the old deadline no longer counts")
			newDeadline := start.Add(time.Duration(tc.newInterval+tc.newGrace) * time.Second)
			assert.False(t, b.overdueAt(t, newDeadline.Add(-time.Second))[h.ID], "not overdue before the new deadline")
			assert.True(t, b.overdueAt(t, newDeadline.Add(time.Second))[h.ID], "overdue right after the new deadline")

			heartbeat.RunDeadlineCheck(ctx, b.svc)
			got, err := b.svc.GetHeartbeat(ctx, h.ID)
			require.NoError(t, err)
			assert.Equal(t, tc.status, got.Status)
			alerts, _ := b.raised()
			assert.Empty(t, alerts)
		})
	}
}
