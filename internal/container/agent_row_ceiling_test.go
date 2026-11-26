// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container_test

import (
	"bytes"
	"context"
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
