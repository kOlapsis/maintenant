// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
)

func TestActivateSwarm_WiresEventsAndAlertsLikeAtBoot(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	a.alertEngine.Start(ctx)
	sse := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(sse)

	dr, ok := a.rt.(*docker.Runtime)
	require.True(t, ok)
	require.Nil(t, a.swarmEvents)
	a.activateSwarm(ctx, dr)

	a.swarmEvents.ProcessEvent(ctx, runtime.RuntimeEvent{
		ResourceType: runtime.ResourceService, Action: "remove", ExternalID: "svc1", Name: "prod_web",
	})
	for range 3 {
		a.swarmCrashLoop.RecordFailure("svc1", "prod_web", "exit 1")
	}

	seen := map[string]bool{}
	require.Eventually(t, func() bool {
		for {
			select {
			case evt := <-sse:
				seen[evt.Type] = true
			default:
				return seen[event.SwarmServiceRemoved] && seen[event.SwarmCrashLoopDetected]
			}
		}
	}, 2*time.Second, 10*time.Millisecond, "the Swarm services built at runtime broadcast their events")

	require.Eventually(t, func() bool {
		active, err := a.alertStore.ListActiveAlerts(ctx)
		require.NoError(t, err)
		for _, al := range active {
			if al.Source == "swarm" && al.AlertType == "crash_loop" && al.EntityID == "svc1" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "the Swarm services built at runtime raise their alerts")
}
