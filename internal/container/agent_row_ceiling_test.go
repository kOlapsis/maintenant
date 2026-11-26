// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type ceilingBench struct {
	t    *testing.T
	ctx  context.Context
	st   *store.ContainerStore
	svc  *container.Service
	logs *bytes.Buffer
}

func newCeilingBench(t *testing.T, agentIDs ...string) *ceilingBench {
	t.Helper()
	ctx := context.Background()
	db := storetest.Open(t, nil)
	for _, id := range agentIDs {
		require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
			AgentID: id, Hostname: id, Label: id, OSArch: "linux/amd64",
			AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
		}))
	}
	logs := &bytes.Buffer{}
	st := store.NewContainerStore(db)
	svc := container.NewService(container.Deps{Store: st, Logger: slog.New(slog.NewTextHandler(logs, nil))})
	return &ceilingBench{t: t, ctx: ctx, st: st, svc: svc, logs: logs}
}

func (b *ceilingBench) report(agentID string, ev *agentpb.ContainerEvent) error {
	return b.svc.HandleAgentEvent(b.ctx, agentID, ev, agentevent.Meta{ObservedAt: time.Now()})
}

func (b *ceilingBench) rows(agentID string) (live, archived int) {
	b.t.Helper()
	all, err := b.st.ListContainers(b.ctx, container.ListContainersOpts{IncludeArchived: true, IncludeIgnored: true, AgentFilter: &agentID})
	require.NoError(b.t, err)
	for _, c := range all {
		if c.Archived {
			archived++
		} else {
			live++
		}
	}
	return live, archived
}

func running(id string) *agentpb.ContainerEvent {
	return &agentpb.ContainerEvent{
		ContainerId: id, Name: "n-" + id, Image: "app:v1",
		State:  agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels: map[string]string{"maintenant.update.track": "minor", "maintenant.typo": "x"},
	}
}

func TestHandleAgentEvent_RefusesNewContainersOnceTheAgentHoldsTheCeiling(t *testing.T) {
	const agentA, agentB = "00000000-0000-0000-0000-0000000000a1", "00000000-0000-0000-0000-0000000000b2"
	container.SetMaxAgentContainers(t, 3)
	b := newCeilingBench(t, agentA, agentB)

	for _, id := range []string{"ceil-1", "ceil-2", "ceil-3"} {
		require.NoError(t, b.report(agentA, running(id)))
	}

	for _, id := range []string{"ceil-4", "ceil-5", "ceil-6"} {
		err := b.report(agentA, running(id))
		require.ErrorIs(t, err, agentevent.ErrInvalid, "a refused container must not be replayed by the agent")
		_, recorded := b.svc.AgentDetails(agentA, id)
		assert.False(t, recorded, "nothing is kept in memory for a refused container")
		assert.False(t, container.UnknownLabelsWarned(id), "no warning entry for a container the server does not hold")
	}
	live, archived := b.rows(agentA)
	assert.Equal(t, 3, live+archived, "the agent's rows stop at the ceiling")
	assert.Equal(t, 1, strings.Count(b.logs.String(), "container ceiling"), "the refusal is logged once per agent")

	stopped := running("ceil-1")
	stopped.State = agentpb.ContainerState_CONTAINER_STATE_EXITED
	stopped.StatusMessage = "1"
	require.NoError(t, b.report(agentA, stopped), "containers the server already holds keep being followed")

	require.NoError(t, b.report(agentB, running("ceil-4")), "another agent is not affected")
}

func TestHandleAgentInventory_StillArchivesWhenTheAgentHoldsTheCeiling(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a3"
	container.SetMaxAgentContainers(t, 3)
	b := newCeilingBench(t, agentA)

	for _, id := range []string{"inv-1", "inv-2", "inv-3"} {
		require.NoError(t, b.report(agentA, running(id)))
	}

	err := b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete:   true,
		Containers: []*agentpb.ContainerEvent{running("inv-9"), running("inv-2"), running("inv-3")},
	}, agentevent.Meta{ObservedAt: time.Now()})
	require.ErrorIs(t, err, agentevent.ErrInvalid)

	live, archived := b.rows(agentA)
	assert.Equal(t, 2, live)
	assert.Equal(t, 1, archived, "a container missing from the inventory is archived even when a new one is refused")
}

func TestHandleAgentEvent_DropsWhatItKeptInMemoryWhenTheContainerGoes(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a4"
	b := newCeilingBench(t, agentA)

	require.NoError(t, b.report(agentA, running("gone-1")))
	require.NoError(t, b.report(agentA, running("gone-2")))
	_, recorded := b.svc.AgentDetails(agentA, "gone-1")
	require.True(t, recorded)
	require.True(t, container.UnknownLabelsWarned("gone-1"))
	require.True(t, container.UnknownLabelsWarned("gone-2"))

	destroyed := running("gone-1")
	destroyed.State = agentpb.ContainerState_CONTAINER_STATE_EXITED
	destroyed.Destroyed = true
	require.NoError(t, b.report(agentA, destroyed))
	_, recorded = b.svc.AgentDetails(agentA, "gone-1")
	assert.False(t, recorded)
	assert.False(t, container.UnknownLabelsWarned("gone-1"))

	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{Complete: true}, agentevent.Meta{ObservedAt: time.Now()}))
	_, recorded = b.svc.AgentDetails(agentA, "gone-2")
	assert.False(t, recorded)
	assert.False(t, container.UnknownLabelsWarned("gone-2"), "a container archived by the inventory is forgotten too")

	never := running("gone-3")
	never.State = agentpb.ContainerState_CONTAINER_STATE_EXITED
	never.Destroyed = true
	require.NoError(t, b.report(agentA, never))
	assert.False(t, container.UnknownLabelsWarned("gone-3"), "a removal of a container the server never held leaves nothing behind")
}

func TestHandleAgentEvent_MakesRoomAmongArchivedContainersAtTheCeiling(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a5"
	container.SetMaxAgentContainers(t, 3)
	b := newCeilingBench(t, agentA)

	for _, id := range []string{"room-1", "room-2", "room-3"} {
		require.NoError(t, b.report(agentA, running(id)))
	}
	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete: true, Containers: []*agentpb.ContainerEvent{running("room-3")},
	}, agentevent.Meta{ObservedAt: time.Now()}))

	require.NoError(t, b.report(agentA, running("room-4")), "a host at its ceiling keeps its new containers")
	require.NoError(t, b.report(agentA, running("room-5")))
	live, archived := b.rows(agentA)
	assert.Equal(t, 3, live)
	assert.Zero(t, archived, "the oldest archived containers made room")
	assert.Zero(t, strings.Count(b.logs.String(), "container ceiling"), "making room is not a refusal")

	require.ErrorIs(t, b.report(agentA, running("room-6")), agentevent.ErrInvalid, "live containers alone at the ceiling are refused")
	live, archived = b.rows(agentA)
	assert.Equal(t, 3, live+archived)
}

func TestHandleAgentInventory_AcceptsNewContainersAtTheCeilingWhenArchivedOnesCanGo(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a6"
	container.SetMaxAgentContainers(t, 3)
	b := newCeilingBench(t, agentA)
	meta := agentevent.Meta{ObservedAt: time.Now()}

	for _, id := range []string{"sync-1", "sync-2", "sync-3"} {
		require.NoError(t, b.report(agentA, running(id)))
	}
	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete: true, Containers: []*agentpb.ContainerEvent{running("sync-3")},
	}, meta))

	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete: true, Containers: []*agentpb.ContainerEvent{running("sync-3"), running("sync-4")},
	}, meta), "an inventory that succeeds is followed by the label sync of its containers")
	live, _ := b.rows(agentA)
	assert.Equal(t, 2, live)
}

func (b *ceilingBench) fill(agentID string, n int) []string {
	b.t.Helper()
	ids := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("c%02d", i+1)
		require.NoError(b.t, b.report(agentID, running(id)))
		ids = append(ids, id)
	}
	return ids
}

func (b *ceilingBench) held(agentID, externalID string) *container.Container {
	b.t.Helper()
	c, err := b.st.GetContainerByExternalID(b.ctx, agentID, externalID)
	require.NoError(b.t, err)
	return c
}

func (b *ceilingBench) archive(agentID, externalID string, ago time.Duration) {
	b.t.Helper()
	require.NoError(b.t, b.st.ArchiveContainer(b.ctx, b.held(agentID, externalID).ID, time.Now().Add(-ago)))
}

func TestHandleAgentEvent_DoesNotEvictWhenRowsWereDeletedSinceTheLastCount(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a7"
	container.SetMaxAgentContainers(t, 30)
	b := newCeilingBench(t, agentA)

	ids := b.fill(agentA, 30)
	for i, id := range ids[:20] {
		b.archive(agentA, id, time.Duration(i+1)*time.Hour)
	}
	for _, id := range ids[25:] {
		require.NoError(t, b.svc.DeleteContainer(b.ctx, b.held(agentA, id).ID))
	}

	require.NoError(t, b.report(agentA, running("new-1")))
	live, archived := b.rows(agentA)
	assert.Equal(t, 20, archived, "there was room, no archived container had to go")
	assert.Equal(t, 6, live)
}

func TestHandleAgentInventory_KeepsAnArchivedContainerItBringsBack(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000a8"
	container.SetMaxAgentContainers(t, 30)
	b := newCeilingBench(t, agentA)

	ids := b.fill(agentA, 30)
	b.archive(agentA, "c01", 72*time.Hour)
	b.archive(agentA, "c02", 48*time.Hour)
	b.archive(agentA, "c03", 24*time.Hour)
	b.archive(agentA, "c04", time.Hour)
	revived := b.held(agentA, "c01").ID
	_, err := b.st.InsertTransition(b.ctx, &container.StateTransition{
		ContainerID: revived, PreviousState: container.StateRunning, NewState: container.StateExited,
		Timestamp: time.Now().Add(-100 * time.Hour),
	})
	require.NoError(t, err)
	_, before, err := b.st.ListTransitionsByContainer(b.ctx, revived, container.ListTransitionsOpts{Limit: 10})
	require.NoError(t, err)

	inv := &agentpb.ContainerInventory{Complete: true, Containers: []*agentpb.ContainerEvent{running("new-1"), running("c01")}}
	for _, id := range ids[4:] {
		inv.Containers = append(inv.Containers, running(id))
	}
	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, inv, agentevent.Meta{ObservedAt: time.Now()}))

	back := b.held(agentA, "c01")
	require.NotNil(t, back)
	_, after, err := b.st.ListTransitionsByContainer(b.ctx, back.ID, container.ListTransitionsOpts{Limit: 10})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, after, before, "a container the inventory brings back keeps its history")
	assert.False(t, back.Archived)
	assert.NotNil(t, b.held(agentA, "new-1"))
	for _, id := range []string{"c02", "c03", "c04"} {
		assert.Nil(t, b.held(agentA, id), "%s made room", id)
	}
}
