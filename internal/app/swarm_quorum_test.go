// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"testing"
	"time"

	dockerswarm "github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/swarm"
)

type readyManagersClient struct{}

func (readyManagersClient) ServiceList(context.Context) ([]dockerswarm.Service, error) {
	return nil, nil
}
func (readyManagersClient) ServiceInspect(context.Context, string) (dockerswarm.Service, error) {
	return dockerswarm.Service{}, nil
}
func (readyManagersClient) TaskList(context.Context) ([]dockerswarm.Task, error) { return nil, nil }
func (readyManagersClient) NodeList(context.Context) ([]dockerswarm.Node, error) {
	var nodes []dockerswarm.Node
	for _, id := range []string{"m1", "m2", "m3"} {
		nodes = append(nodes, dockerswarm.Node{
			ID:          id,
			Spec:        dockerswarm.NodeSpec{Role: dockerswarm.NodeRoleManager, Availability: dockerswarm.NodeAvailabilityActive},
			Status:      dockerswarm.NodeStatus{State: dockerswarm.NodeStateReady},
			Description: dockerswarm.NodeDescription{Hostname: id},
		})
	}
	return nodes, nil
}

func TestSeedSwarmAlertTracking_ResolvesAQuorumAlertOnceTheQuorumIsBack(t *testing.T) {
	a, ctx := newKubernetesAlertApp(t)
	_, err := a.alertStore.InsertAlert(ctx, &alert.Alert{
		Source:     "swarm",
		AlertType:  "quorum_degraded",
		Severity:   alert.SeverityCritical,
		Status:     alert.StatusActive,
		Message:    "quorum degraded",
		EntityType: "swarm_cluster",
		EntityName: "swarm",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)
	a.alertEngine.Start(ctx)

	client := readyManagersClient{}
	discovery := swarm.NewServiceDiscovery(client, a.logger)
	m := &swarmManager{
		discovery:      discovery,
		events:         swarm.NewEventProcessor(discovery, a.logger),
		nodeSvc:        swarm.NewNodeService(client, a.swarmNodeStore, a.logger),
		crashLoop:      swarm.NewCrashLoopDetector(a.logger),
		updateTracker:  swarm.NewUpdateTracker(client, a.logger),
		replicaChecker: swarm.NewReplicaHealthChecker(a.logger),
	}
	a.wireSwarmCallbacks(m)
	a.seedSwarmAlertTracking(ctx, m)

	require.NoError(t, m.nodeSvc.Reconcile(ctx))
	require.Eventually(t, func() bool {
		active, err := a.alertStore.ListActiveAlerts(ctx)
		require.NoError(t, err)
		return len(active) == 0
	}, 2*time.Second, 10*time.Millisecond, "a quorum back after a restart resolves the alert raised before it")
}
