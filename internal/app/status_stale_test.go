// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/uid"
)

type answeringRuntime struct{ runtime.Runtime }

func (answeringRuntime) IsConnected() bool { return true }

func containerMonitorStatus(t *testing.T, a *App, c *container.Container) string {
	t.Helper()
	ctx := context.Background()
	c.ExternalID, c.Name, c.AlertSeverity, c.RuntimeType = uid.New(), "web", container.SeverityWarning, "docker"
	if c.LastStateChangeAt.IsZero() {
		c.LastStateChangeAt = time.Now()
	}
	c.FirstSeenAt = c.LastStateChangeAt
	id, err := a.containerStore.InsertContainer(ctx, c)
	require.NoError(t, err)
	return a.statusSvc.DeriveComponentStatus(ctx, &status.Component{Monitors: []status.MonitorRef{{Type: "container", ID: id}}})
}

func TestStatusProvider_ContainerStatusFollowsWhatTheRuntimeVouchesFor(t *testing.T) {
	a, _ := newTestApp(t, nil)
	running := func(agentID string) *container.Container {
		return &container.Container{AgentID: agentID, State: container.StateRunning}
	}

	assert.Equal(t, status.StatusMajorOutage, containerMonitorStatus(t, a, running("")), "the local runtime does not answer")

	a.rt = answeringRuntime{a.rt}
	assert.Equal(t, status.StatusOperational, containerMonitorStatus(t, a, running("")))
	assert.Equal(t, status.StatusOperational, containerMonitorStatus(t, a, &container.Container{State: container.StateCreated}), "a container that is starting")
	assert.Equal(t, status.StatusMajorOutage, containerMonitorStatus(t, a, &container.Container{
		State: container.StateCreated, LastStateChangeAt: time.Now().Add(-10 * time.Minute),
	}), "a container that never started")

}
