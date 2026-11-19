// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

var errStoreDown = errors.New("store down")

type flakyStore struct {
	*memStore
	errGet, errUpdate, errInsert error
}

func (f *flakyStore) GetEndpointByID(ctx context.Context, id string) (*Endpoint, error) {
	if f.errGet != nil {
		return nil, f.errGet
	}
	return f.memStore.GetEndpointByID(ctx, id)
}

func (f *flakyStore) UpdateCheckResult(ctx context.Context, id string, st EndpointStatus, as AlertState, cf, cs int, rt int64, hs *int, le string) error {
	if f.errUpdate != nil {
		return f.errUpdate
	}
	return f.memStore.UpdateCheckResult(ctx, id, st, as, cf, cs, rt, hs, le)
}

func (f *flakyStore) InsertCheckResult(ctx context.Context, r *CheckResult) (string, error) {
	if f.errInsert != nil {
		return "", f.errInsert
	}
	if r.ID == "" {
		r.ID = uid.New()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, old := range f.results {
		if old.ID == r.ID {
			cp := *r
			f.results[i] = &cp
			return r.ID, nil
		}
	}
	cp := *r
	f.results = append(f.results, &cp)
	return r.ID, nil
}

func agentProbe(target string) *agentpb.EndpointEvent {
	return &agentpb.EndpointEvent{Url: target, Status: agentpb.EndpointStatus_ENDPOINT_STATUS_UP, StatusCode: 200, LatencyMs: 12}
}

func seededAgentEndpointService(t *testing.T) (*Service, *flakyStore, string) {
	t.Helper()
	store := &flakyStore{memStore: newMemStore()}
	seedEndpoint(t, store.memStore, &Endpoint{
		ContainerName: "web", LabelKey: "0", Target: "http://web/", AgentID: "agent-1",
		EndpointType: TypeHTTP, Status: StatusUnknown,
	})
	svc := NewService(Deps{Store: store, Engine: noopEngine(), Logger: noopLogger()})
	return svc, store, "http://web/"
}

func TestHandleAgentEvent_StoreFailuresAreReturned(t *testing.T) {
	for name, set := range map[string]func(*flakyStore){
		"endpoint lookup":     func(f *flakyStore) { f.errGet = errStoreDown },
		"endpoint state":      func(f *flakyStore) { f.errUpdate = errStoreDown },
		"check result insert": func(f *flakyStore) { f.errInsert = errStoreDown },
	} {
		t.Run(name, func(t *testing.T) {
			svc, store, target := seededAgentEndpointService(t)
			set(store)

			err := svc.HandleAgentEvent(context.Background(), "agent-1", agentProbe(target),
				agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1"})
			require.ErrorIs(t, err, errStoreDown, "a probe result the store refused must not read as handled")
		})
	}
}

func TestHandleAgentEvent_ReplayedStoreFailureIsReturned(t *testing.T) {
	svc, store, target := seededAgentEndpointService(t)
	store.errInsert = errStoreDown

	err := svc.HandleAgentEvent(context.Background(), "agent-1", agentProbe(target),
		agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1", Replayed: true})
	require.ErrorIs(t, err, errStoreDown)
}

func TestHandleAgentEvent_ReplayAfterFailureStoresTheResultOnce(t *testing.T) {
	svc, store, target := seededAgentEndpointService(t)
	meta := agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1"}

	store.errInsert = errStoreDown
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", agentProbe(target), meta), errStoreDown)
	store.errInsert = nil

	meta.Replayed = true
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", agentProbe(target), meta))
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", agentProbe(target), meta))

	require.Len(t, store.results, 1, "the replay of one event writes one row")
	assert.Equal(t, uid.EventRecord("agent-1", "evt-1", "check_result"), store.results[0].ID)
}
