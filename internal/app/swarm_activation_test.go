// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/swarm"
)

func TestActivateSwarm_WiresEventsAndAlertsLikeAtBoot(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	a.alertEngine.Start(ctx)
	sse := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(sse)

	dr, ok := a.rt.(*docker.Runtime)
	require.True(t, ok)
	require.Nil(t, a.swarmMgr.Load())
	a.activateSwarm(ctx, dr)
	m := a.swarmMgr.Load()
	require.NotNil(t, m)

	m.events.ProcessEvent(ctx, runtime.RuntimeEvent{
		ResourceType: runtime.ResourceService, Action: "remove", ExternalID: "svc1", Name: "prod_web",
	})
	for range 3 {
		m.crashLoop.RecordFailure("svc1", "prod_web", "exit 1")
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

	assert.Same(t, m.updateTracker, a.currentSwarmUpdateTracker(), "the API reads the tracker built at runtime")
	assert.Same(t, m.crashLoop, a.currentSwarmCrashLoop())
	assert.Same(t, m.discovery, a.currentSwarmDiscovery())
}

func TestStartSwarmManager_ResumesAlertsLeftByThePreviousRun(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	_, err := a.alertStore.InsertAlert(ctx, &alert.Alert{
		Source:     "swarm",
		AlertType:  "replica_unhealthy",
		Severity:   alert.SeverityWarning,
		Status:     alert.StatusActive,
		Message:    "under-replicated",
		EntityType: "swarm_service",
		EntityID:   "svc1",
		EntityName: "prod_web",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)
	a.alertEngine.Start(ctx)

	dr, ok := a.rt.(*docker.Runtime)
	require.True(t, ok)
	a.activateSwarm(ctx, dr)
	a.swarmMgr.Load().replicaChecker.Check([]*swarm.SwarmService{
		{ServiceID: "svc1", Name: "prod_web", Mode: "replicated", DesiredReplicas: 2, RunningReplicas: 2},
	})

	require.Eventually(t, func() bool {
		active, err := a.alertStore.ListActiveAlerts(ctx)
		require.NoError(t, err)
		return len(active) == 0
	}, 2*time.Second, 10*time.Millisecond, "a service healthy again after a restart resolves the alert raised before it")
}

func TestSwarmContextChanges_AreSafeWhileEventsFlow(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	a.alertEngine.Start(ctx)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		died := runtime.RuntimeEvent{
			Action: "die", ExternalID: "task-container",
			Labels: map[string]string{"com.docker.swarm.service.id": "svc1", "com.docker.swarm.service.name": "prod_web"},
		}
		updated := runtime.RuntimeEvent{ResourceType: runtime.ResourceService, Action: "remove", ExternalID: "svc1"}
		for {
			select {
			case <-stop:
				return
			default:
			}
			a.dispatchRuntimeEvent(ctx, updated)
			a.dispatchRuntimeEvent(ctx, died)
			_ = a.swarmCluster.Load()
			_ = a.currentSwarmDiscovery()
			_ = a.currentSwarmUpdateTracker()
			_ = a.currentSwarmCrashLoop()
		}
	}()

	a.applySwarmContext(ctx, swarm.DetectionResult{Active: true, IsManager: true, ClusterID: "cluster-1"})
	a.applySwarmContext(ctx, swarm.DetectionResult{})
	a.applySwarmContext(ctx, swarm.DetectionResult{Active: true, IsManager: true, ClusterID: "cluster-1"})
	close(stop)
	wg.Wait()

	require.NotNil(t, a.swarmMgr.Load())
	require.NotNil(t, a.swarmCluster.Load())
	assert.Equal(t, "cluster-1", a.swarmCluster.Load().ID)
}
