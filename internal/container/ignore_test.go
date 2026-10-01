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

func TestIgnoredByLabels(t *testing.T) {
	assert.True(t, IgnoredByLabels(map[string]string{"maintenant.ignore": "true"}))
	assert.True(t, IgnoredByLabels(map[string]string{"maintenant.ignore": "1"}))
	assert.False(t, IgnoredByLabels(map[string]string{"maintenant.ignore": "false"}))
	assert.False(t, IgnoredByLabels(nil))
}

func TestOrchestrationGroupFromLabels(t *testing.T) {
	assert.Equal(t, "shop", OrchestrationGroupFromLabels(map[string]string{
		"com.docker.compose.project": "shop", "com.docker.stack.namespace": "prod",
	}))
	assert.Equal(t, "prod", OrchestrationGroupFromLabels(map[string]string{"com.docker.stack.namespace": "prod"}))
	assert.Empty(t, OrchestrationGroupFromLabels(nil))
}

func TestService_IgnoredContainerRaisesNothing(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("ignored"), StateExited)
	c.ID = "ign-1"
	c.IsIgnored = true
	healthy := HealthHealthy
	c.HealthStatus = &healthy
	store.seed(c)

	checker := &mockRestartChecker{result: map[string]interface{}{"restarts": 10}}
	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) { emitted = append(emitted, eventType) }
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))
	unhealthy := makeTestEvent("health_status", c.ExternalID)
	unhealthy.HealthStatus = string(HealthUnhealthy)
	svc.ProcessEvent(context.Background(), unhealthy)

	assert.Empty(t, emitted, "an ignored container must not feed the alert pipeline")
	assert.Zero(t, checker.calls, "no restart check for an ignored container")
}

// Swarm service labels and Kubernetes annotations change on a container that
// is already stored; the next reconcile must pick them up.
func TestService_Reconcile_AdoptsLabelFields(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("stack"), StateRunning)
	c.ID = "stk-1"
	c.AlertSeverity = SeverityWarning
	c.RestartThreshold = 3
	store.seed(c)

	discovered := makeTestContainer(c.ExternalID, StateExited)
	discovered.IsIgnored = true
	discovered.OrchestrationGroup = "prod"
	discovered.CustomGroup = "payments"
	discovered.AlertSeverity = SeverityCritical
	discovered.RestartThreshold = 7

	var emitted []string
	discoverer := &mockDiscoverer{containers: []*Container{discovered}}
	svc := newTestService(store, func(d *Deps) {
		d.Discoverer = discoverer
		d.EventCallback = func(eventType string, _ interface{}) { emitted = append(emitted, eventType) }
	})

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	got, err := store.GetContainerByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.True(t, got.IsIgnored)
	assert.Equal(t, "prod", got.OrchestrationGroup)
	assert.Equal(t, "payments", got.CustomGroup)
	assert.Equal(t, SeverityCritical, got.AlertSeverity)
	assert.Equal(t, 7, got.RestartThreshold)
	assert.NotContains(t, emitted, "container.state_changed", "an ignored container announces no state change")
}

func TestService_Reconcile_IgnoredNewContainerIsNotAnnounced(t *testing.T) {
	store := newSvcStore()
	discovered := makeTestContainer(extID("newign"), StateRunning)
	discovered.IsIgnored = true

	var emitted []string
	discoverer := &mockDiscoverer{containers: []*Container{discovered}}
	svc := newTestService(store, func(d *Deps) {
		d.Discoverer = discoverer
		d.EventCallback = func(eventType string, _ interface{}) { emitted = append(emitted, eventType) }
	})

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	assert.NotContains(t, emitted, "container.discovered")
}

func TestHandleAgentEvent_AdoptsLabelFields(t *testing.T) {
	store := newSvcStore()
	id := extID("agentstack")
	c := makeTestContainer(id, StateRunning)
	c.ID = "ag-1"
	c.AgentID = "agent-1"
	c.AlertSeverity = SeverityWarning
	c.RestartThreshold = 3
	store.seed(c)

	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.EventCallback = func(eventType string, _ interface{}) { emitted = append(emitted, eventType) }
	})

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        c.Name,
		State:       agentpb.ContainerState_CONTAINER_STATE_EXITED,
		Labels: map[string]string{
			"maintenant.ignore":          "true",
			"com.docker.stack.namespace": "prod",
		},
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev, agentevent.Meta{ObservedAt: time.Now()}))

	got, err := store.GetContainerByExternalID(context.Background(), "agent-1", id)
	require.NoError(t, err)
	assert.True(t, got.IsIgnored)
	assert.Equal(t, "prod", got.OrchestrationGroup)
	assert.Equal(t, StateRunning, got.State, "the exit of an ignored container is not processed")
	assert.Empty(t, emitted)
}

func TestHandleAgentEvent_NewContainerGroupedByStack(t *testing.T) {
	store := newSvcStore()
	id := extID("agentnew")
	svc := newTestService(store)

	ev := &agentpb.ContainerEvent{
		ContainerId: id,
		Name:        "prod_web.1.abc",
		State:       agentpb.ContainerState_CONTAINER_STATE_RUNNING,
		Labels:      map[string]string{"com.docker.stack.namespace": "prod"},
	}
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev, agentevent.Meta{ObservedAt: time.Now()}))

	got, err := store.GetContainerByExternalID(context.Background(), "agent-1", id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "prod", got.OrchestrationGroup)
}
