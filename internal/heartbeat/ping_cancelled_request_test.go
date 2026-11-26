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

type cancelAfterCommitStore struct {
	heartbeat.HeartbeatStore
	cancel context.CancelFunc
}

func (s *cancelAfterCommitStore) TransitionHeartbeatState(ctx context.Context, id string, fromStatus heartbeat.HeartbeatStatus, fromDeadline *time.Time,
	status heartbeat.HeartbeatStatus, alertState heartbeat.AlertState,
	lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
	lastExitCode *int, lastDurationMs *int64,
	consecutiveFailures, consecutiveSuccesses int) (bool, error) {
	won, err := s.HeartbeatStore.TransitionHeartbeatState(ctx, id, fromStatus, fromDeadline, status, alertState,
		lastPingAt, nextDeadlineAt, currentRunStartedAt, lastExitCode, lastDurationMs, consecutiveFailures, consecutiveSuccesses)
	s.cancel()
	return won, err
}

func TestPing_IsFullyRecordedWhenTheRequestIsCancelledAfterTheStateCommit(t *testing.T) {
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
			h := b.seed(t, heartbeat.StatusUp, time.Now().Add(time.Hour))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			svc := heartbeat.NewService(heartbeat.Deps{
				Store:  &cancelAfterCommitStore{HeartbeatStore: b.real, cancel: cancel},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})

			require.NoError(t, tc.ping(ctx, svc, h.PingToken))
			require.Error(t, ctx.Err())

			pings, _, err := b.real.ListPings(context.Background(), h.ID, heartbeat.ListPingsOpts{Limit: 10})
			require.NoError(t, err)
			assert.Len(t, pings, 1)
			execs, _, err := b.real.ListExecutions(context.Background(), h.ID, heartbeat.ListExecutionsOpts{Limit: 10})
			require.NoError(t, err)
			assert.Len(t, execs, 1)
		})
	}
}
