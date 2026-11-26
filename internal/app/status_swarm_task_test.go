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
	"github.com/kolapsis/maintenant/internal/status"
)

func TestStatusProvider_ReplacedSwarmTaskLeavesTheMatchAllPopulation(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, nil)
	addTask := func(name string, state container.ContainerState) {
		_, err := a.containerStore.InsertContainer(ctx, &container.Container{
			ExternalID: name, Name: name, State: state, AlertSeverity: container.SeverityWarning, RuntimeType: "docker",
			ControllerKind: container.ControllerSwarmService, SwarmServiceID: "svc-web", SwarmTaskSlot: 1,
			FirstSeenAt: time.Now(), LastStateChangeAt: time.Now(),
		})
		require.NoError(t, err)
	}
	derive := func() string {
		return a.statusSvc.DeriveComponentStatus(ctx, &status.Component{CompositionMode: status.CompositionMatchAll, MatchAllType: "container"})
	}

	addTask("web.1.old", container.StateExited)
	assert.Equal(t, status.StatusMajorOutage, derive(), "no task of the service runs")

	addTask("web.1.new", container.StateRunning)
	assert.Equal(t, status.StatusOperational, derive(), "the failed task was replaced")
}
