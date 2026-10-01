// Copyright 2026 Benjamin Touchard (Kolapsis)
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

func swarmTaskLabels(slot string) map[string]string {
	return map[string]string{
		"com.docker.swarm.service.id":   "svc1",
		"com.docker.swarm.service.name": "prod_web",
		"com.docker.swarm.node.id":      "node1",
		"com.docker.swarm.task":         "",
		"com.docker.swarm.task.id":      "task9",
		"com.docker.swarm.task.name":    "prod_web." + slot + ".task9",
		"com.docker.stack.namespace":    "prod",
	}
}

func TestApplySwarmTaskLabels(t *testing.T) {
	var replicated Container
	replicated.ApplySwarmTaskLabels(swarmTaskLabels("2"))
	assert.Equal(t, ControllerSwarmService, replicated.ControllerKind)
	assert.Equal(t, "svc1", replicated.SwarmServiceID)
	assert.Equal(t, "prod_web", replicated.SwarmServiceName)
	assert.Equal(t, "node1", replicated.SwarmNodeID)
	assert.Equal(t, 2, replicated.SwarmTaskSlot)

	var global Container
	global.ApplySwarmTaskLabels(swarmTaskLabels("node1"))
	assert.Equal(t, "svc1", global.SwarmServiceID)
	assert.Zero(t, global.SwarmTaskSlot, "a global service's task names its node, not a slot")

	var standalone Container
	standalone.ApplySwarmTaskLabels(map[string]string{"com.docker.compose.service": "web"})
	assert.Empty(t, standalone.ControllerKind)
	assert.Empty(t, standalone.SwarmServiceID)
	assert.Zero(t, standalone.SwarmTaskSlot)
}

func TestGroupSource_SwarmTaskIsNotANamespace(t *testing.T) {
	var c Container
	c.RuntimeType = "docker"
	c.OrchestrationGroup = OrchestrationGroupFromLabels(swarmTaskLabels("1"))
	c.ApplySwarmTaskLabels(swarmTaskLabels("1"))
	assert.Equal(t, "compose", c.GroupSource())

	k8s := Container{RuntimeType: "kubernetes", ControllerKind: "Deployment", OrchestrationGroup: "shop"}
	assert.Equal(t, "namespace", k8s.GroupSource())
}

func TestHandleAgentEvent_FillsTheSwarmService(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("swarm-task")
	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "prod_web.2.task9",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels:      swarmTaskLabels("2"),
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", ev, agentevent.Meta{ObservedAt: time.Now()}))

	c, _ := store.GetContainerByExternalID(context.Background(), "a", id)
	require.NotNil(t, c)
	assert.Equal(t, ControllerSwarmService, c.ControllerKind)
	assert.Equal(t, "svc1", c.SwarmServiceID)
	assert.Equal(t, "prod_web", c.SwarmServiceName)
	assert.Equal(t, "node1", c.SwarmNodeID)
	assert.Equal(t, 2, c.SwarmTaskSlot)
}

func TestHandleAgentEvent_RefreshFillsTheSwarmService(t *testing.T) {
	store := newSvcStore()
	svc := newTestService(store)

	id := extID("swarm-known")
	known := makeTestContainer(id, StateRunning)
	known.ID = "known-1"
	known.AgentID = "a"
	store.seed(known)

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        known.Name,
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels:      swarmTaskLabels("3"),
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "a", ev, agentevent.Meta{ObservedAt: time.Now()}))

	got, err := store.GetContainerByID(context.Background(), known.ID)
	require.NoError(t, err)
	assert.Equal(t, ControllerSwarmService, got.ControllerKind)
	assert.Equal(t, "svc1", got.SwarmServiceID)
	assert.Equal(t, 3, got.SwarmTaskSlot)
}

func TestService_Reconcile_FillsTheSwarmService(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("local-task"), StateRunning)
	c.ID = "task-1"
	store.seed(c)

	discovered := makeTestContainer(c.ExternalID, StateRunning)
	discovered.ApplySwarmTaskLabels(swarmTaskLabels("1"))
	discoverer := &mockDiscoverer{containers: []*Container{discovered}}
	svc := newTestService(store, func(d *Deps) { d.Discoverer = discoverer })

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	got, err := store.GetContainerByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, ControllerSwarmService, got.ControllerKind)
	assert.Equal(t, "prod_web", got.SwarmServiceName)
	assert.Equal(t, 1, got.SwarmTaskSlot)
}
