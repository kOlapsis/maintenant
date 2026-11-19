// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

var errStoreDown = errors.New("store down")

type flakyStore struct {
	*mockStore
	errLookup, errTransition error
}

func (f *flakyStore) GetHeartbeatByPingToken(ctx context.Context, token string) (*Heartbeat, error) {
	if f.errLookup != nil {
		return nil, f.errLookup
	}
	return f.mockStore.GetHeartbeatByPingToken(ctx, token)
}

func (f *flakyStore) TransitionHeartbeatState(ctx context.Context, id string, fromStatus HeartbeatStatus, fromDeadline *time.Time,
	status HeartbeatStatus, alertState AlertState, lastPingAt, nextDeadlineAt, currentRunStartedAt *time.Time,
	lastExitCode *int, lastDurationMs *int64, cf, cs int) (bool, error) {
	if f.errTransition != nil {
		return false, f.errTransition
	}
	return f.mockStore.TransitionHeartbeatState(ctx, id, fromStatus, fromDeadline, status, alertState,
		lastPingAt, nextDeadlineAt, currentRunStartedAt, lastExitCode, lastDurationMs, cf, cs)
}

func flakyAgentService(t *testing.T) (*Service, *flakyStore) {
	t.Helper()
	base := newMockStore()
	seedHeartbeat(base, "tok", StatusNew, AlertNormal)
	store := &flakyStore{mockStore: base}
	svc := NewService(Deps{
		Store:          store,
		Logger:         slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		LicenseChecker: &mockLicense{canCreate: true},
	})
	return svc, store
}

func TestHandleAgentEvent_StoreFailuresAreReturned(t *testing.T) {
	for name, set := range map[string]func(*flakyStore){
		"heartbeat lookup": func(f *flakyStore) { f.errLookup = errStoreDown },
		"state update":     func(f *flakyStore) { f.errTransition = errStoreDown },
	} {
		t.Run(name, func(t *testing.T) {
			svc, store := flakyAgentService(t)
			set(store)
			err := svc.HandleAgentEvent(context.Background(), "agent-1", &agentpb.HeartbeatEvent{HeartbeatToken: "tok"})
			require.ErrorIs(t, err, errStoreDown, "a ping the store refused must not read as handled")
		})
	}
}

func TestHandleAgentEvent_ReplayAfterFailureCountsThePingOnce(t *testing.T) {
	svc, store := flakyAgentService(t)
	ev := &agentpb.HeartbeatEvent{HeartbeatToken: "tok"}

	store.errTransition = errStoreDown
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev), errStoreDown)
	store.errTransition = nil
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev))

	assert.Len(t, store.pings, 1, "a ping that never changed the state leaves no history row behind")
	h, err := store.GetHeartbeatByID(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, 1, h.ConsecutiveSuccesses)
}
