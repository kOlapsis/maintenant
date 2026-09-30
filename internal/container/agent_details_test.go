// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

func TestAgentDetails_KeepsUpdateLabelsAndDigestsFromTheInventory(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	ctx := context.Background()
	const agentID = "agent-d"
	id := extID("web")

	_, known := svc.AgentDetails(agentID, id)
	assert.False(t, known, "nothing is known before the agent reports the container")

	inv := &agentpb.ContainerInventory{Complete: true, Containers: []*agentpb.ContainerEvent{{
		ContainerId: id, Name: "web", Image: "nginx:latest", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels: map[string]string{
			"maintenant.update.track":    "patch",
			"com.docker.compose.project": "shop",
		},
		RepoDigests: &agentpb.RepoDigests{Digests: []string{"nginx@sha256:running"}},
	}}}
	require.NoError(t, svc.HandleAgentInventory(ctx, agentID, inv, agentevent.Meta{ObservedAt: time.Now()}))

	d, known := svc.AgentDetails(agentID, id)
	require.True(t, known)
	assert.Equal(t, map[string]string{"maintenant.update.track": "patch"}, d.UpdateLabels)
	assert.Equal(t, []string{"nginx@sha256:running"}, d.RepoDigests)
	assert.True(t, d.DigestsKnown)

	_, known = svc.AgentDetails("another-agent", id)
	assert.False(t, known)

	restart := &agentpb.ContainerEvent{ContainerId: id, Name: "web", Image: "nginx:latest",
		State: agentpb.ContainerState_CONTAINER_STATE_RESTARTING, Labels: map[string]string{"maintenant.update.track": "patch"}}
	require.NoError(t, svc.HandleAgentEvent(ctx, agentID, restart, agentevent.Meta{ObservedAt: time.Now()}))
	d, _ = svc.AgentDetails(agentID, id)
	assert.Equal(t, []string{"nginx@sha256:running"}, d.RepoDigests, "a lifecycle event without digests keeps the known ones")

	require.NoError(t, svc.HandleAgentEvent(ctx, agentID, &agentpb.ContainerEvent{ContainerId: id, Destroyed: true,
		State: agentpb.ContainerState_CONTAINER_STATE_EXITED}, agentevent.Meta{ObservedAt: time.Now()}))
	_, known = svc.AgentDetails(agentID, id)
	assert.False(t, known, "a destroyed container is forgotten")
}

func TestAgentDetails_LocallyBuiltImageAndArchivedContainer(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	ctx := context.Background()
	const agentID = "agent-e"
	built, gone := extID("built"), extID("gone")

	entry := func(id string) *agentpb.ContainerEvent {
		return &agentpb.ContainerEvent{ContainerId: id, Name: id[:5], Image: "app:dev",
			State: agentpb.ContainerState_CONTAINER_STATE_RUNNING, RepoDigests: &agentpb.RepoDigests{}}
	}
	require.NoError(t, svc.HandleAgentInventory(ctx, agentID, &agentpb.ContainerInventory{Complete: true,
		Containers: []*agentpb.ContainerEvent{entry(built), entry(gone)}}, agentevent.Meta{ObservedAt: time.Now()}))

	d, known := svc.AgentDetails(agentID, built)
	require.True(t, known)
	assert.True(t, d.DigestsKnown)
	assert.Empty(t, d.RepoDigests, "an empty list says the image never came from a registry")

	require.NoError(t, svc.HandleAgentInventory(ctx, agentID, &agentpb.ContainerInventory{Complete: true,
		Containers: []*agentpb.ContainerEvent{entry(built)}}, agentevent.Meta{ObservedAt: time.Now()}))
	_, known = svc.AgentDetails(agentID, gone)
	assert.False(t, known, "a container the inventory archives is forgotten")
}

func TestAgentDetails_ReplayedEventsAreNotRecorded(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)
	id := extID("old")

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-r", &agentpb.ContainerEvent{
		ContainerId: id, Name: "old", Image: "nginx:1", State: agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		RepoDigests: &agentpb.RepoDigests{Digests: []string{"nginx@sha256:stale"}},
	}, agentevent.Meta{ObservedAt: time.Now(), Replayed: true}))

	_, known := svc.AgentDetails("agent-r", id)
	assert.False(t, known)
}
