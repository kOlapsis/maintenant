// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dockerswarm "github.com/moby/moby/api/types/swarm"
	dockersystem "github.com/moby/moby/api/types/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/swarm"
)

func crashingService() swarm.TopologySnapshot {
	snap := swarm.TopologySnapshot{Services: []swarm.SwarmService{{ServiceID: "svc1", Name: "prod_web", Mode: "replicated", DesiredReplicas: 1}}}
	code := 1
	for _, id := range []string{"t1", "t2", "t3"} {
		snap.Tasks = append(snap.Tasks, swarm.SwarmTask{
			TaskID: id, ServiceID: "svc1", NodeID: "n2", State: "failed", DesiredState: "shutdown", ExitCode: &code, Timestamp: time.Now(),
		})
	}
	return snap
}

type swarmInfo struct{ info dockerswarm.Info }

func (s swarmInfo) Info(context.Context) (dockersystem.Info, error) {
	return dockersystem.Info{Swarm: s.info}, nil
}

func TestNew_ArmsSwarmDetectionWhileDockerIsUnreachable(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	require.False(t, a.rt.IsConnected())
	require.NotNil(t, a.swarmDetector, "Docker not answering at startup still leaves Swarm detection armed")

	a.swarmDetector = swarm.NewDetector(swarmInfo{info: dockerswarm.Info{
		LocalNodeState: dockerswarm.LocalNodeStateActive, ControlAvailable: true, Nodes: 5, Managers: 3,
		Cluster: &dockerswarm.ClusterInfo{ID: "cluster-1"},
	}}, a.logger)
	go a.startSwarmRecheck(ctx)
	a.wireContainerMonitoring(ctx)

	require.Eventually(t, func() bool { return a.swarmMgr.Load() != nil }, 2*time.Second, 10*time.Millisecond,
		"a manager is detected once the runtime connects, not a recheck period later")
	cluster := a.swarmCluster.Load()
	require.NotNil(t, cluster)
	assert.Equal(t, "cluster-1", cluster.ID)
	assert.Equal(t, 3, cluster.ManagerCount)
	assert.Equal(t, 2, cluster.WorkerCount)
}

func TestApplySwarmDetection_KeepsTheClusterCountsCurrent(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	sse := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(sse)

	a.applySwarmDetection(ctx, swarm.DetectionResult{Active: true})
	assert.Nil(t, a.swarmCluster.Load())
	assert.Empty(t, sse, "a worker was never a Swarm context: nothing switches")

	a.swarmCluster.Store(&swarm.SwarmCluster{ID: "cluster-1", IsManager: true})
	a.applySwarmDetection(ctx, swarm.DetectionResult{Active: true, IsManager: true, ClusterID: "cluster-1", ManagerCount: 3, WorkerCount: 4})
	cluster := a.swarmCluster.Load()
	require.NotNil(t, cluster)
	assert.Equal(t, 3, cluster.ManagerCount)
	assert.Equal(t, 4, cluster.WorkerCount)
	for _, evt := range drain(sse) {
		assert.NotEqual(t, event.RuntimeContextChanged, evt.Type, "a manager staying a manager does not switch the context")
	}
}

func drain(sse chan v1.SSEEvent) []v1.SSEEvent {
	var out []v1.SSEEvent
	for {
		select {
		case evt := <-sse:
			out = append(out, evt)
		default:
			return out
		}
	}
}

func TestApplySwarmDetection_AnnouncesEachChangeOfTheSwarmState(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	sse := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(sse)
	manager := swarm.DetectionResult{Active: true, IsManager: true, ClusterID: "cluster-1", ManagerCount: 3, WorkerCount: 4}

	a.applySwarmDetection(ctx, manager)
	a.applySwarmDetection(ctx, manager)
	manager.WorkerCount = 5
	a.applySwarmDetection(ctx, manager)
	a.applySwarmDetection(ctx, swarm.DetectionResult{Active: true})
	a.applySwarmDetection(ctx, swarm.DetectionResult{})

	var statuses []any
	for _, evt := range drain(sse) {
		if evt.Type == event.SwarmStatus {
			statuses = append(statuses, evt.Data)
		}
	}
	assert.Equal(t, []any{
		map[string]any{"active": true, "is_manager": true, "cluster_id": "cluster-1", "manager_count": 3, "worker_count": 4},
		map[string]any{"active": true, "is_manager": true, "cluster_id": "cluster-1", "manager_count": 3, "worker_count": 5},
		map[string]any{"active": false},
	}, statuses)
}

func TestSwarmDeactivation_StopsTheManagerUntilSwarmReturns(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	manager := swarm.DetectionResult{Active: true, IsManager: true, ClusterID: "cluster-1"}

	a.applySwarmDetection(ctx, manager)
	m := a.swarmMgr.Load()
	require.NotNil(t, m)

	a.applySwarmDetection(ctx, swarm.DetectionResult{})
	assert.Nil(t, a.swarmMgr.Load(), "no Swarm manager outlives the cluster")
	assert.Nil(t, a.currentSwarmDiscovery())
	stopped := make(chan struct{})
	go func() {
		m.loops.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the loops of the deactivated manager still run")
	}

	a.applySwarmDetection(ctx, manager)
	again := a.swarmMgr.Load()
	require.NotNil(t, again)
	assert.NotSame(t, m, again, "a reactivation starts a new manager")
}

func TestDispatchRuntimeEvent_TaskContainersStoppingAreNoCrash(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	dr, ok := a.rt.(*docker.Runtime)
	require.True(t, ok)
	a.activateSwarm(ctx, dr)
	m := a.swarmMgr.Load()
	require.NotNil(t, m)

	task := map[string]string{"com.docker.swarm.service.id": "svc1", "com.docker.swarm.service.name": "prod_web"}
	for i := range 3 {
		a.dispatchRuntimeEvent(ctx, runtime.RuntimeEvent{
			Action: "die", ExternalID: fmt.Sprintf("task-%d", i), ExitCode: "143", Labels: task, Timestamp: time.Now(),
		})
	}
	assert.False(t, m.crashLoop.IsCrashLooping("svc1"), "a rolling update stopping three tasks is not a crash loop")
}

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
	m.crashLoop.ObserveTasks(crashingService())

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
