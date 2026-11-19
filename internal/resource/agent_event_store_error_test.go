// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

var errStoreDown = errors.New("store down")

type upsertingStore struct {
	*mockResourceStore
	errInsert error
	rows      map[string]*ResourceSnapshot
}

func (u *upsertingStore) InsertSnapshot(_ context.Context, s *ResourceSnapshot) (string, error) {
	if u.errInsert != nil {
		return "", u.errInsert
	}
	if s.ID == "" {
		s.ID = uid.New()
	}
	if u.rows == nil {
		u.rows = map[string]*ResourceSnapshot{}
	}
	cp := *s
	u.rows[s.ID] = &cp
	return s.ID, nil
}

func agentResourceFixture(t *testing.T) (*Service, *upsertingStore, string) {
	t.Helper()
	extID := "abc123def4567890"
	c := &container.Container{ID: uid.Container(uid.Agent("agent-9"), extID), ExternalID: extID, AgentID: "agent-9", Name: "demo"}
	store := &upsertingStore{mockResourceStore: newMockResourceStore()}
	return newTestService(store, buildContainerSvc(newMockContainerStore(c)), nil), store, extID
}

func TestHandleAgentEvent_SnapshotStoreFailureIsReturned(t *testing.T) {
	for _, replayed := range []bool{false, true} {
		svc, store, extID := agentResourceFixture(t)
		store.errInsert = errStoreDown

		err := svc.HandleAgentEvent(context.Background(), "agent-9", &agentpb.ResourceSample{ContainerId: extID, CpuPercent: 3},
			agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1", Replayed: replayed})
		require.ErrorIs(t, err, errStoreDown, "a sample the store refused must not read as handled (replayed=%v)", replayed)
	}
}

func TestHandleAgentEvent_ReplayAfterFailureStoresTheSampleOnce(t *testing.T) {
	svc, store, extID := agentResourceFixture(t)
	meta := agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1"}
	sample := &agentpb.ResourceSample{ContainerId: extID, CpuPercent: 3}

	store.errInsert = errStoreDown
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-9", sample, meta), errStoreDown)
	store.errInsert = nil

	meta.Replayed = true
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-9", sample, meta))
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-9", sample, meta))

	require.Len(t, store.rows, 1, "the replay of one event writes one row")
	assert.Contains(t, store.rows, uid.EventRecord(uid.Agent("agent-9"), "evt-1", "resource_snapshot"))
}
