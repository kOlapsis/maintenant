// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
)

func drainTopologyChanges(sse <-chan v1.SSEEvent) []v1.SSEEvent {
	var out []v1.SSEEvent
	for {
		select {
		case evt := <-sse:
			if evt.Type == "kubernetes.topology_changed" {
				out = append(out, evt)
			}
		default:
			return out
		}
	}
}

func TestKubernetesReconcile_BroadcastsTopologyChangeOnlyWhenItChanged(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	sse := make(chan v1.SSEEvent, 256)
	a.broker.AddObserver(sse)

	prev := localTopologyReconcileInterval
	localTopologyReconcileInterval = 10 * time.Millisecond
	t.Cleanup(func() { localTopologyReconcileInterval = prev })

	api := kubernetes.K8sWorkload{ID: "shop/Deployment/api", Name: "api", Namespace: "shop", Kind: "Deployment", DesiredReplicas: 2, ReadyReplicas: 2}
	cluster := &fakeCluster{}
	cluster.set([]kubernetes.K8sWorkload{api}, nil)

	until := make(chan struct{})
	done := make(chan struct{})
	go func() {
		a.startKubernetesReconcile(ctx, cluster, until)
		close(done)
	}()
	t.Cleanup(func() {
		close(until)
		<-done
	})

	var changes []v1.SSEEvent
	require.Eventually(t, func() bool {
		changes = append(changes, drainTopologyChanges(sse)...)
		return len(changes) == 1
	}, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, map[string]any{"agent_id": uid.LocalAgent}, changes[0].Data)

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, drainTopologyChanges(sse), "an unchanged cluster is not announced again")

	api.ReadyReplicas = 0
	cluster.set([]kubernetes.K8sWorkload{api}, nil)
	require.Eventually(t, func() bool {
		return len(drainTopologyChanges(sse)) == 1
	}, 2*time.Second, 5*time.Millisecond)
}
