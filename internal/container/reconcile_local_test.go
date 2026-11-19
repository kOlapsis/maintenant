// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type eventLog struct {
	mu     sync.Mutex
	events map[string]int
}

func newEventLog() *eventLog { return &eventLog{events: map[string]int{}} }

func (l *eventLog) record(eventType string, _ interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events[eventType]++
}

func (l *eventLog) count(eventType string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.events[eventType]
}

func withHealth(c *Container, h HealthStatus) *Container {
	c.HealthStatus = &h
	return c
}

func healthTransitions(s *svcStore, containerID string) []*StateTransition {
	var out []*StateTransition
	for _, t := range s.transitionsFor(containerID) {
		if t.NewHealth != nil {
			out = append(out, t)
		}
	}
	return out
}

func TestReconcile_AdoptsHealthChangedWithoutEvent(t *testing.T) {
	store := newSvcStore()
	c := withHealth(makeTestContainer(extID("h1"), StateRunning), HealthHealthy)
	c.ID = "60"
	store.seed(c)

	discovered := withHealth(makeTestContainer(c.ExternalID, StateRunning), HealthUnhealthy)
	discoverer := &mockDiscoverer{containers: []*Container{discovered}}
	log := newEventLog()
	svc := newTestService(store, func(d *Deps) { d.EventCallback = log.record })

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))
	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	h := store.storedHealthStatus(c.ExternalID)
	require.NotNil(t, h)
	assert.Equal(t, HealthUnhealthy, *h)
	assert.Equal(t, 1, log.count("container.health_changed"), "the health change is announced once, not at each tick")
	assert.Len(t, healthTransitions(store, c.ID), 1)
}

func TestReconcile_AdoptsHealthOfAContainerWithNone(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("h2"), StateRunning)
	c.ID = "61"
	store.seed(c)

	discoverer := &mockDiscoverer{containers: []*Container{withHealth(makeTestContainer(c.ExternalID, StateRunning), HealthUnhealthy)}}
	log := newEventLog()
	svc := newTestService(store, func(d *Deps) { d.EventCallback = log.record })

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	h := store.storedHealthStatus(c.ExternalID)
	require.NotNil(t, h)
	assert.Equal(t, HealthUnhealthy, *h)
	assert.Equal(t, 1, log.count("container.health_changed"))
}

func TestReconcile_HealthEventAfterReconcileIsNotAnnouncedTwice(t *testing.T) {
	store := newSvcStore()
	c := withHealth(makeTestContainer(extID("h3"), StateRunning), HealthHealthy)
	c.ID = "62"
	store.seed(c)

	discoverer := &mockDiscoverer{containers: []*Container{withHealth(makeTestContainer(c.ExternalID, StateRunning), HealthUnhealthy)}}
	log := newEventLog()
	svc := newTestService(store, func(d *Deps) { d.EventCallback = log.record })

	require.NoError(t, svc.Reconcile(context.Background(), discoverer))
	evt := makeTestEvent("health_status", c.ExternalID)
	evt.HealthStatus = string(HealthUnhealthy)
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, 1, log.count("container.health_changed"))
	assert.Len(t, healthTransitions(store, c.ID), 1)
}

func TestReconcile_ConcurrentHealthEventsAndTicksAnnounceOnce(t *testing.T) {
	store := newSvcStore()
	c := withHealth(makeTestContainer(extID("h4"), StateRunning), HealthHealthy)
	c.ID = "63"
	store.seed(c)

	discoverer := &mockDiscoverer{containers: []*Container{withHealth(makeTestContainer(c.ExternalID, StateRunning), HealthUnhealthy)}}
	log := newEventLog()
	svc := newTestService(store, func(d *Deps) { d.EventCallback = log.record })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			evt := makeTestEvent("health_status", c.ExternalID)
			evt.HealthStatus = string(HealthUnhealthy)
			svc.ProcessEvent(context.Background(), evt)
		}()
		go func() {
			defer wg.Done()
			_ = svc.Reconcile(context.Background(), discoverer)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, log.count("container.health_changed"))
	assert.Len(t, healthTransitions(store, c.ID), 1)
}

func TestReconcile_StateLeftWrongByALostUpdateIsFixedOnce(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("s1"), StateExited)
	c.ID = "64"
	store.seed(c)

	discoverer := &mockDiscoverer{containers: []*Container{makeTestContainer(c.ExternalID, StateRunning)}}
	log := newEventLog()
	svc := newTestService(store, func(d *Deps) { d.EventCallback = log.record })

	store.errUpdate = errors.New("database is locked")
	require.NoError(t, svc.Reconcile(context.Background(), discoverer))
	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
	assert.Equal(t, 0, log.count("container.state_changed"))

	store.errUpdate = nil
	require.NoError(t, svc.Reconcile(context.Background(), discoverer))
	require.NoError(t, svc.Reconcile(context.Background(), discoverer))

	assert.Equal(t, StateRunning, store.storedState(c.ExternalID))
	assert.Equal(t, 1, log.count("container.state_changed"))
	assert.Len(t, store.transitionsFor(c.ID), 1, "a failed attempt leaves no transition behind")
}

func TestReconcile_AdoptsReplicaCounts(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("k1"), StateRunning)
	c.ID = "65"
	c.PodCount, c.ReadyCount = 3, 3
	store.seed(c)

	discovered := makeTestContainer(c.ExternalID, StateRunning)
	discovered.PodCount, discovered.ReadyCount = 3, 1
	discovered.ErrorDetail = "partial: 1/3 ready"
	svc := newTestService(store)

	require.NoError(t, svc.Reconcile(context.Background(), &mockDiscoverer{containers: []*Container{discovered}}))

	got, err := store.GetContainerByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.PodCount)
	assert.Equal(t, 1, got.ReadyCount)
	assert.Equal(t, "partial: 1/3 ready", got.ErrorDetail)
}

func TestReconcile_ControllerScaledToZeroLeavesRunning(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("k2"), StateRunning)
	c.ID = "66"
	c.PodCount, c.ReadyCount = 2, 2
	store.seed(c)

	discovered := makeTestContainer(c.ExternalID, StateCreated)
	discovered.PodCount, discovered.ReadyCount = 2, 0
	svc := newTestService(store)

	require.NoError(t, svc.Reconcile(context.Background(), &mockDiscoverer{containers: []*Container{discovered}}))

	got, err := store.GetContainerByID(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, StateCreated, got.State)
	assert.Equal(t, 0, got.ReadyCount)
}
