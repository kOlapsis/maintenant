// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

// ClusterHealthHealthy indicates all nodes are ready and all services fully replicated.
const ClusterHealthHealthy = "healthy"

// ClusterHealthDegraded indicates some nodes are drained/paused or services are under-replicated.
const ClusterHealthDegraded = "degraded"

// ClusterHealthUnhealthy indicates nodes are down/disconnected or services have zero running replicas.
const ClusterHealthUnhealthy = "unhealthy"

// ComputeClusterHealth computes the overall cluster health from services and nodes.
// Priority: unhealthy > degraded > healthy.
func ComputeClusterHealth(services []*SwarmService, nodes []*SwarmNode) string {
	// Check for unhealthy conditions first.
	for _, n := range nodes {
		if n.Status == "down" || n.Status == "disconnected" {
			return ClusterHealthUnhealthy
		}
	}
	for _, s := range services {
		if s.Mode == "replicated" && s.DesiredReplicas > 0 && s.RunningReplicas == 0 {
			return ClusterHealthUnhealthy
		}
	}

	// Check for degraded conditions.
	for _, n := range nodes {
		if n.Availability == "drain" || n.Availability == "pause" {
			return ClusterHealthDegraded
		}
	}
	for _, s := range services {
		if s.Mode == "replicated" && s.DesiredReplicas > 0 && s.RunningReplicas < s.DesiredReplicas {
			return ClusterHealthDegraded
		}
	}

	return ClusterHealthHealthy
}
