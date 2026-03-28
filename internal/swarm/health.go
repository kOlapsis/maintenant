// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
