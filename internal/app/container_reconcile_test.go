// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

type observedRuntime struct {
	runtime.Runtime
	observed atomic.Pointer[container.Container]
}

func (r *observedRuntime) DiscoverAll(context.Context) ([]*container.Container, error) {
	c := *r.observed.Load()
	return []*container.Container{&c}, nil
}

func TestContainerReconcile_FollowsTheRuntimeWithoutEvents(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	healthy, unhealthy := container.HealthHealthy, container.HealthUnhealthy
	seed := &container.Container{
		ExternalID: "web", Name: "web", Image: "web:1", State: container.StateRunning, HealthStatus: &healthy,
		RuntimeType: "docker", AlertSeverity: container.SeverityWarning, RestartThreshold: 3, AgentID: uid.LocalAgent, FirstSeenAt: time.Now(), LastStateChangeAt: time.Now(),
	}
	containers := store.NewContainerStore(a.db)
	id, err := containers.InsertContainer(ctx, seed)
	require.NoError(t, err)

	rt := &observedRuntime{}
	observed := *seed
	observed.HealthStatus = &unhealthy
	rt.observed.Store(&observed)
	a.rt = rt

	prev := containerReconcileInterval
	containerReconcileInterval = 10 * time.Millisecond
	t.Cleanup(func() { containerReconcileInterval = prev })

	until := make(chan struct{})
	done := make(chan struct{})
	go func() {
		a.startContainerReconcile(ctx, until)
		close(done)
	}()

	require.Eventually(t, func() bool {
		c, err := containers.GetContainerByID(ctx, id)
		return err == nil && c != nil && c.HealthStatus != nil && *c.HealthStatus == container.HealthUnhealthy
	}, 2*time.Second, 10*time.Millisecond)

	close(until)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "the loop did not stop when the connection cycle ended")
	}
}
