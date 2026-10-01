// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeCluster struct {
	mu        sync.Mutex
	workloads []kubernetes.K8sWorkload
	pods      []kubernetes.K8sPod
}

func (f *fakeCluster) set(workloads []kubernetes.K8sWorkload, pods []kubernetes.K8sPod) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workloads, f.pods = workloads, pods
}

func (f *fakeCluster) ListNamespaces(context.Context) ([]string, error) {
	return []string{"shop"}, nil
}

func (f *fakeCluster) ListWorkloads(context.Context, []string) ([]kubernetes.K8sWorkloadGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []kubernetes.K8sWorkloadGroup{{Namespace: "shop", Workloads: f.workloads}}, nil
}

func (f *fakeCluster) ListPods(context.Context, []string, kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pods, nil
}

func (f *fakeCluster) ListNodes(context.Context) ([]kubernetes.K8sNode, error) {
	return []kubernetes.K8sNode{{Name: "node-1", Status: "ready"}}, nil
}

func (f *fakeCluster) ListAllEvents(context.Context) ([]kubernetes.K8sEventRef, error) {
	return nil, nil
}

func newKubernetesAlertApp(t *testing.T) (*App, context.Context) {
	t.Helper()
	cfg, logger := downWiringConfig(t, 0)
	a, err := New(cfg, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a.db.StartWriter(ctx)
	return a, ctx
}

func reconcileCluster(ctx context.Context, a *App, src kubernetes.SnapshotSource) {
	stop := make(chan struct{})
	close(stop)
	a.startKubernetesReconcile(ctx, src, stop)
}

func activeKubernetesAlerts(t *testing.T, ctx context.Context, a *App) []*alert.Alert {
	t.Helper()
	active, err := a.alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	var out []*alert.Alert
	for _, al := range active {
		if al.Source == "kubernetes" {
			out = append(out, al)
		}
	}
	return out
}

func TestKubernetesReconcile_RaisesAndResolvesAlerts(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	a.alertEngine.Start(ctx)
	sse := make(chan v1.SSEEvent, 64)
	a.broker.AddObserver(sse)

	cluster := &fakeCluster{}
	cluster.set(nil, []kubernetes.K8sPod{{Name: "api-x", Namespace: "shop", Status: "Running", StatusReason: "CrashLoopBackOff", RestartCount: 5}})
	reconcileCluster(ctx, a, cluster)

	require.Eventually(t, func() bool { return len(activeKubernetesAlerts(t, ctx, a)) == 1 }, 2*time.Second, 10*time.Millisecond)
	fired := activeKubernetesAlerts(t, ctx, a)[0]
	assert.Equal(t, "crash_loop", fired.AlertType)
	assert.Equal(t, "pod", fired.EntityType)
	assert.Equal(t, "shop/api-x", fired.EntityID)
	assert.Equal(t, alert.SeverityCritical, fired.Severity)
	assert.Equal(t, uid.LocalAgent, fired.AgentID)

	seen := map[string]bool{}
	require.Eventually(t, func() bool {
		for {
			select {
			case evt := <-sse:
				seen[evt.Type] = true
			default:
				return seen[event.KubernetesPodChanged] && seen[event.AlertFired]
			}
		}
	}, 2*time.Second, 10*time.Millisecond)

	cluster.set(nil, []kubernetes.K8sPod{{Name: "api-x", Namespace: "shop", Status: "Running", RestartCount: 5}})
	reconcileCluster(ctx, a, cluster)
	require.Eventually(t, func() bool { return len(activeKubernetesAlerts(t, ctx, a)) == 0 }, 2*time.Second, 10*time.Millisecond)
}

func TestSeedKubernetesAlertTracking_ResolvesAfterRestart(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	_, err := a.alertStore.InsertAlert(ctx, &alert.Alert{
		Source:     "kubernetes",
		AlertType:  "replica_health",
		Severity:   alert.SeverityWarning,
		Status:     alert.StatusActive,
		Message:    "under-replicated",
		EntityType: "workload",
		EntityID:   "shop/Deployment/api",
		EntityName: "shop/Deployment/api",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)
	a.alertEngine.Start(ctx)
	a.seedKubernetesAlertTracking(ctx)

	cluster := &fakeCluster{}
	cluster.set([]kubernetes.K8sWorkload{{
		ID: "shop/Deployment/api", Name: "api", Namespace: "shop", Kind: "Deployment", ReadyReplicas: 3, DesiredReplicas: 3,
	}}, nil)
	reconcileCluster(ctx, a, cluster)

	require.Eventually(t, func() bool { return len(activeKubernetesAlerts(t, ctx, a)) == 0 }, 2*time.Second, 10*time.Millisecond,
		"a workload healthy again after a restart resolves the alert raised before it")
}
