// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package container

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
)

// mockAgentRuntime implements AgentRuntimeResolver for tests.
type mockAgentRuntime struct {
	runtime string
	err     error
}

func (m *mockAgentRuntime) DetectedRuntime(_ context.Context, _ string) (string, error) {
	return m.runtime, m.err
}

type capturedEvent struct {
	typ  string
	data interface{}
}

func captureEvents(into *[]capturedEvent) func(*Deps) {
	return func(d *Deps) {
		d.EventCallback = func(typ string, data interface{}) {
			*into = append(*into, capturedEvent{typ: typ, data: data})
		}
	}
}

func hasEvent(events []capturedEvent, typ string) bool {
	for _, e := range events {
		if e.typ == typ {
			return true
		}
	}
	return false
}

func TestHandleAgentEvent_InsertsNewContainer(t *testing.T) {
	store := newSvcStore()
	var events []capturedEvent
	svc := newTestService(store, captureEvents(&events), func(d *Deps) {
		d.AgentRuntime = &mockAgentRuntime{runtime: "docker"}
	})

	id := extID("demo-app")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "demo-app",
		Image:       "adminer:latest",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels: map[string]string{
			labelComposeProject:    "demo",
			labelComposeService:    "app",
			labelComposeWorkingDir: "/home/demo",
		},
	}

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-123", ev))

	c, err := store.GetContainerByExternalID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "agent-123", c.AgentID)
	assert.Equal(t, "adminer:latest", c.Image)
	assert.Equal(t, StateRunning, c.State)
	assert.Equal(t, "demo", c.OrchestrationGroup)
	assert.Equal(t, "app", c.OrchestrationUnit)
	assert.Equal(t, "/home/demo", c.ComposeWorkingDir)
	assert.Equal(t, "docker", c.RuntimeType)
	assert.Equal(t, 1, c.ReadyCount)

	txns := store.transitionsFor(c.ID)
	require.Len(t, txns, 1)
	assert.Equal(t, StateCreated, txns[0].PreviousState)
	assert.Equal(t, StateRunning, txns[0].NewState)

	assert.True(t, hasEvent(events, event.ContainerDiscovered), "expected ContainerDiscovered event")
}

func TestHandleAgentEvent_InsertsNonRunningState(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("stopped")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "stopped",
		State:       agentpb.ContainerState_CONTAINER_STATE_EXITED,
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev))

	c, err := store.GetContainerByExternalID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, c, "exited container from agent must still be inserted")
	assert.Equal(t, StateExited, c.State)
	assert.Equal(t, 0, c.ReadyCount)
	// fallback runtime "docker" when no resolver is wired
	assert.Equal(t, "docker", c.RuntimeType)
}

func TestHandleAgentEvent_CreatedStateNoInitialTransition(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("created")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "created",
		State:       agentpb.ContainerState_CONTAINER_STATE_CREATED,
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev))

	c, _ := store.GetContainerByExternalID(context.Background(), id)
	require.NotNil(t, c)
	assert.Equal(t, StateCreated, c.State)
	assert.Empty(t, store.transitionsFor(c.ID), "created state should not record an initial transition")
}

func TestHandleAgentEvent_UpdatesExistingState(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	agentID := "agent-1"
	id := extID("svc")
	seed := makeTestContainer(id, StateRunning)
	seed.AgentID = agentID
	store.seed(seed)

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "svc",
		State:       agentpb.ContainerState_CONTAINER_STATE_EXITED,
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), agentID, ev))

	assert.Equal(t, StateExited, store.storedState(id))
	c, _ := store.GetContainerByExternalID(context.Background(), id)
	require.NotNil(t, c)
	assert.Equal(t, agentID, c.AgentID)
}

func TestHandleAgentEvent_BackfillsAgentID(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("orphan")
	seed := makeTestContainer(id, StateRunning) // AgentID nil
	store.seed(seed)

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "orphan",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-77", ev))

	c, _ := store.GetContainerByExternalID(context.Background(), id)
	require.NotNil(t, c)
	assert.Equal(t, "agent-77", c.AgentID, "agent_id should be backfilled on existing row")
}

func TestHandleAgentEvent_UpdatesImageOnRedeploy(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	agentID := "agent-1"
	id := extID("redeploy")
	seed := makeTestContainer(id, StateRunning)
	seed.AgentID = agentID
	seed.Image = "app:v1"
	store.seed(seed)

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "redeploy",
		Image:       "app:v2",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), agentID, ev))

	c, _ := store.GetContainerByExternalID(context.Background(), id)
	require.NotNil(t, c)
	assert.Equal(t, "app:v2", c.Image)
}

func TestHandleAgentEvent_RuntimeResolver(t *testing.T) {
	t.Run("resolves agent runtime", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store, func(d *Deps) {
			d.AgentRuntime = &mockAgentRuntime{runtime: "swarm"}
		})
		id := extID("swarmsvc")
		require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", &agentpb.ContainerEvent{
			ContainerId: id, Name: "swarmsvc", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		}))
		c, _ := store.GetContainerByExternalID(context.Background(), id)
		require.NotNil(t, c)
		assert.Equal(t, "swarm", c.RuntimeType)
	})

	t.Run("falls back to docker on resolver error", func(t *testing.T) {
		store := newSvcStore()
		svc := newTestService(store, func(d *Deps) {
			d.AgentRuntime = &mockAgentRuntime{err: errors.New("resolver down")}
		})
		id := extID("errsvc")
		require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", &agentpb.ContainerEvent{
			ContainerId: id, Name: "errsvc", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		}))
		c, _ := store.GetContainerByExternalID(context.Background(), id)
		require.NotNil(t, c)
		assert.Equal(t, "docker", c.RuntimeType)
	})
}

func TestHandleAgentEvent_AppliesMaintenantLabels(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("labeled")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "labeled",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels: map[string]string{
			labelPBIgnore:    "true",
			labelPBGroup:     "infra",
			labelPBSeverity:  "critical",
			labelPBThreshold: "5",
			labelPBChannels:  "ops",
		},
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", ev))

	c, _ := store.GetContainerByExternalID(context.Background(), id)
	require.NotNil(t, c)
	assert.True(t, c.IsIgnored)
	assert.Equal(t, "infra", c.CustomGroup)
	assert.Equal(t, SeverityCritical, c.AlertSeverity)
	assert.Equal(t, 5, c.RestartThreshold)
	assert.Equal(t, "ops", c.AlertChannels)
}

func TestHandleAgentEvent_EmptyContainerIDIsNoOp(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", &agentpb.ContainerEvent{
		ContainerId: "", Name: "x", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
	}))
	got, err := store.ListContainers(context.Background(), ListContainersOpts{IncludeArchived: true})
	require.NoError(t, err)
	assert.Empty(t, got, "events without a container id must be ignored")
}

func TestHandleAgentEvent_LiveLifecycleRunningThenCompleted(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	ctx := context.Background()
	id := extID("lifecycle")

	// 1) inventory: running → insert
	require.NoError(t, svc.HandleAgentEvent(ctx, "agent-1", &agentpb.ContainerEvent{
		ContainerId: id, Name: "lifecycle", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
	}))
	assert.Equal(t, StateRunning, store.storedState(id))

	// 2) live: exited with graceful exit code carried in StatusMessage → completed
	require.NoError(t, svc.HandleAgentEvent(ctx, "agent-1", &agentpb.ContainerEvent{
		ContainerId: id, Name: "lifecycle",
		State:         agentpb.ContainerState_CONTAINER_STATE_EXITED,
		StatusMessage: "0",
	}))
	assert.Equal(t, StateCompleted, store.storedState(id))
}

func TestHandleAgentEvent_DieNonZeroExitBecomesExited(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	ctx := context.Background()
	id := extID("crash")

	agentID := "agent-1"
	seed := makeTestContainer(id, StateRunning)
	seed.AgentID = agentID
	store.seed(seed)

	// Exit code 1 is a genuine crash (137/143 would be graceful → completed).
	require.NoError(t, svc.HandleAgentEvent(ctx, agentID, &agentpb.ContainerEvent{
		ContainerId: id, Name: "crash",
		State:         agentpb.ContainerState_CONTAINER_STATE_EXITED,
		StatusMessage: "1",
	}))
	assert.Equal(t, StateExited, store.storedState(id))
}

func TestHandleAgentEvent_PreservesImageWhenEventImageEmpty(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	ctx := context.Background()
	id := extID("noimg")

	agentID := "agent-1"
	seed := makeTestContainer(id, StateRunning)
	seed.AgentID = agentID
	seed.Image = "keep:me"
	store.seed(seed)

	// Live event without an image must not blank the stored image.
	require.NoError(t, svc.HandleAgentEvent(ctx, agentID, &agentpb.ContainerEvent{
		ContainerId: id, Name: "noimg", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
	}))
	c, _ := store.GetContainerByExternalID(ctx, id)
	require.NotNil(t, c)
	assert.Equal(t, "keep:me", c.Image)
}

func TestHandleAgentEvent_EmitsStateChangedWithAgentID(t *testing.T) {
	store := newSvcStore()
	var events []capturedEvent
	svc := newTestService(store, captureEvents(&events))
	ctx := context.Background()
	id := extID("emits")

	agentID := "agent-xyz"
	seed := makeTestContainer(id, StateRunning)
	seed.AgentID = agentID
	store.seed(seed)

	require.NoError(t, svc.HandleAgentEvent(ctx, agentID, &agentpb.ContainerEvent{
		ContainerId: id, Name: "emits", State: agentpb.ContainerState_CONTAINER_STATE_EXITED,
	}))

	var found bool
	for _, e := range events {
		if e.typ != event.ContainerStateChanged {
			continue
		}
		data, ok := e.data.(map[string]interface{})
		require.True(t, ok)
		aid, ok := data["agent_id"].(string)
		require.True(t, ok, "agent_id should be a string in the event payload")
		assert.Equal(t, agentID, aid)
		found = true
	}
	assert.True(t, found, "expected a ContainerStateChanged event carrying agent_id")
}

func TestContainerStateToState(t *testing.T) {
	cases := []struct {
		in   agentpb.ContainerState
		want ContainerState
	}{
		{agentpb.ContainerState_CONTAINER_STATE_RUNNING, StateRunning},
		{agentpb.ContainerState_CONTAINER_STATE_EXITED, StateExited},
		{agentpb.ContainerState_CONTAINER_STATE_PAUSED, StatePaused},
		{agentpb.ContainerState_CONTAINER_STATE_RESTARTING, StateRestarting},
		{agentpb.ContainerState_CONTAINER_STATE_DEAD, StateDead},
		{agentpb.ContainerState_CONTAINER_STATE_CREATED, StateCreated},
		{agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED, StateRunning},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, containerStateToState(tc.in))
	}
}
