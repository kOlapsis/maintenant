// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
)

type timelineEntry struct {
	from, to container.ContainerState
	at       int64
}

func (b *ceilingBench) timeline(agentID, externalID string) []timelineEntry {
	b.t.Helper()
	c := b.held(agentID, externalID)
	require.NotNil(b.t, c)
	transitions, err := b.st.GetTransitionsInWindow(b.ctx, c.ID, time.Unix(0, 0), time.Now().Add(time.Hour))
	require.NoError(b.t, err)
	out := make([]timelineEntry, 0, len(transitions))
	for _, tr := range transitions {
		out = append(out, timelineEntry{tr.PreviousState, tr.NewState, tr.Timestamp.Unix()})
	}
	return out
}

func exited(id, code string) *agentpb.ContainerEvent {
	ev := running(id)
	ev.State = agentpb.ContainerState_CONTAINER_STATE_EXITED
	ev.StatusMessage = code
	return ev
}

func TestReplay_ATransitionTheInventoryFoundFirstKeepsTheTimeItWasObserved(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000f1"
	b := newCeilingBench(t, agentA)
	now := time.Now().Truncate(time.Second)
	started, died, reconnected := now.Add(-time.Hour), now.Add(-10*time.Minute), now

	require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentA, running("web"), agentevent.Meta{ObservedAt: started, EventID: "start"}))

	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete: true, Containers: []*agentpb.ContainerEvent{exited("web", "1")},
	}, agentevent.Meta{ObservedAt: reconnected, EventID: "inventory"}))

	require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentA, exited("web", "1"),
		agentevent.Meta{ObservedAt: died, Replayed: true, EventID: "die"}))

	assert.Equal(t, []timelineEntry{
		{container.StateCreated, container.StateRunning, started.Unix()},
		{container.StateRunning, container.StateExited, died.Unix()},
	}, b.timeline(agentA, "web"), "the stop is recorded once, at the time the agent saw it")
	assert.Equal(t, died.Unix(), b.held(agentA, "web").LastStateChangeAt.Unix())
}

func TestReplay_KeepsEveryChangeTheInventoryCouldNotSee(t *testing.T) {
	const agentA = "00000000-0000-0000-0000-0000000000f2"
	b := newCeilingBench(t, agentA)
	now := time.Now().Truncate(time.Second)
	started := now.Add(-time.Hour)
	replay := func(ev *agentpb.ContainerEvent, at time.Time, id string) {
		t.Helper()
		require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentA, ev, agentevent.Meta{ObservedAt: at, Replayed: true, EventID: id}))
	}

	require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentA, running("web"), agentevent.Meta{ObservedAt: started, EventID: "start"}))
	require.NoError(t, b.svc.HandleAgentInventory(b.ctx, agentA, &agentpb.ContainerInventory{
		Complete: true, Containers: []*agentpb.ContainerEvent{exited("web", "1")},
	}, agentevent.Meta{ObservedAt: now, EventID: "inventory"}))

	replay(exited("web", "1"), now.Add(-10*time.Minute), "die-1")
	replay(running("web"), now.Add(-5*time.Minute), "start-2")
	replay(exited("web", "1"), now.Add(-2*time.Minute), "die-2")

	assert.Equal(t, []timelineEntry{
		{container.StateCreated, container.StateRunning, started.Unix()},
		{container.StateRunning, container.StateExited, now.Add(-10 * time.Minute).Unix()},
		{container.StateExited, container.StateRunning, now.Add(-5 * time.Minute).Unix()},
		{container.StateRunning, container.StateExited, now.Add(-2 * time.Minute).Unix()},
	}, b.timeline(agentA, "web"))
	assert.Equal(t, now.Add(-2*time.Minute).Unix(), b.held(agentA, "web").LastStateChangeAt.Unix())
}
