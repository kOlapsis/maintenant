// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
)

var errStoreDown = errors.New("store down")

func seededAgentContainer(store *svcStore, state ContainerState, health *HealthStatus) *Container {
	c := makeTestContainer(extID("web"), state)
	c.AgentID = "agent-1"
	c.HealthStatus = health
	store.seed(c)
	return c
}

func agentReport(c *Container, state agentpb.ContainerState, health string) *agentpb.ContainerEvent {
	return &agentpb.ContainerEvent{ContainerId: c.ExternalID, Name: c.Name, Image: c.Image, State: state, HealthStatus: health}
}

func TestHandleAgentEvent_StateChangeStoreErrorIsReturned(t *testing.T) {
	store := newSvcStore()
	c := seededAgentContainer(store, StateRunning, nil)
	store.errUpdate = errStoreDown
	svc := newTestService(store)

	err := svc.HandleAgentEvent(context.Background(), "agent-1",
		agentReport(c, agentpb.ContainerState_CONTAINER_STATE_EXITED, ""), agentevent.Meta{ObservedAt: time.Now()})
	require.ErrorIs(t, err, errStoreDown, "a state change the store refused must not read as handled")
}

func TestHandleAgentEvent_HealthChangeStoreErrorIsReturned(t *testing.T) {
	store := newSvcStore()
	healthy := HealthHealthy
	c := seededAgentContainer(store, StateRunning, &healthy)
	store.errUpdate = errStoreDown
	svc := newTestService(store)

	err := svc.HandleAgentEvent(context.Background(), "agent-1",
		agentReport(c, agentpb.ContainerState_CONTAINER_STATE_RUNNING, string(HealthUnhealthy)), agentevent.Meta{ObservedAt: time.Now()})
	require.ErrorIs(t, err, errStoreDown, "a health change the store refused must not read as handled")
}

func TestHandleAgentEvent_TransitionStoreErrorIsReturnedAfterTheAnnouncement(t *testing.T) {
	store := newSvcStore()
	c := seededAgentContainer(store, StateRunning, nil)
	store.errInsertTransition = errStoreDown
	var events []capturedEvent
	svc := newTestService(store, captureEvents(&events))

	err := svc.HandleAgentEvent(context.Background(), "agent-1",
		agentReport(c, agentpb.ContainerState_CONTAINER_STATE_EXITED, ""), agentevent.Meta{ObservedAt: time.Now(), EventID: "evt-1"})
	require.ErrorIs(t, err, errStoreDown, "the missing timeline row must be written again on replay")
	assert.True(t, hasEvent(events, event.ContainerStateChanged), "the live change is announced once, a replay never announces")
}

func TestHandleAgentEvent_DestroyStoreErrorIsReturned(t *testing.T) {
	store := newSvcStore()
	c := seededAgentContainer(store, StateRunning, nil)
	store.errArchive = errStoreDown
	svc := newTestService(store)

	ev := agentReport(c, agentpb.ContainerState_CONTAINER_STATE_EXITED, "")
	ev.Destroyed = true
	require.ErrorIs(t, svc.HandleAgentEvent(context.Background(), "agent-1", ev, agentevent.Meta{ObservedAt: time.Now()}), errStoreDown)
}
