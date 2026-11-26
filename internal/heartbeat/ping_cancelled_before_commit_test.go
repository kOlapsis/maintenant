// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
)

type cancelBeforeCommitStore struct {
	heartbeat.HeartbeatStore
	cancel  context.CancelFunc
	bounded bool
}

func (s *cancelBeforeCommitStore) GetHeartbeatByPingToken(ctx context.Context, token string) (*heartbeat.Heartbeat, error) {
	h, err := s.HeartbeatStore.GetHeartbeatByPingToken(ctx, token)
	s.cancel()
	return h, err
}

func (s *cancelBeforeCommitStore) TransitionHeartbeatState(ctx context.Context, id string, fromStatus heartbeat.HeartbeatStatus, fromDeadline *time.Time,
	status heartbeat.HeartbeatStatus, alertState heartbeat.AlertState,
	lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
	lastExitCode *int, lastDurationMs *int64,
	consecutiveFailures, consecutiveSuccesses int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, s.bounded = ctx.Deadline()
	return s.HeartbeatStore.TransitionHeartbeatState(ctx, id, fromStatus, fromDeadline, status, alertState,
		lastPingAt, nextDeadlineAt, currentRunStartedAt, lastExitCode, lastDurationMs, consecutiveFailures, consecutiveSuccesses)
}

func TestPing_IsCommittedWhenTheClientHangsUpBeforeTheCommit(t *testing.T) {
	cases := []struct {
		name string
		ping func(ctx context.Context, svc *heartbeat.Service, token string) error
	}{
		{"success ping", func(ctx context.Context, svc *heartbeat.Service, token string) error {
			_, err := svc.ProcessPing(ctx, token, "10.0.0.1", "GET", nil, nil)
			return err
		}},
		{"start ping", func(ctx context.Context, svc *heartbeat.Service, token string) error {
			_, err := svc.ProcessStartPing(ctx, token, "10.0.0.1", "GET")
			return err
		}},
		{"exit code ping", func(ctx context.Context, svc *heartbeat.Service, token string) error {
			_, err := svc.ProcessExitCodePing(ctx, token, 0, "10.0.0.1", "GET", nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newRaceBench(t)
			deadline := time.Now().Add(time.Hour)
			h := b.seed(t, heartbeat.StatusUp, deadline)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			st := &cancelBeforeCommitStore{HeartbeatStore: b.real, cancel: cancel}
			svc := heartbeat.NewService(heartbeat.Deps{Store: st, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

			require.NoError(t, tc.ping(ctx, svc, h.PingToken))
			require.Error(t, ctx.Err())
			assert.True(t, st.bounded, "a stuck store must not hold the ping for ever")

			got, err := b.real.GetHeartbeatByID(context.Background(), h.ID)
			require.NoError(t, err)
			require.NotNil(t, got.NextDeadlineAt)
			assert.True(t, got.NextDeadlineAt.After(deadline), "the deadline is pushed back")
			pings, _, err := b.real.ListPings(context.Background(), h.ID, heartbeat.ListPingsOpts{Limit: 10})
			require.NoError(t, err)
			assert.Len(t, pings, 1)
		})
	}
}
