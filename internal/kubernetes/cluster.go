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

package kubernetes

import (
	"context"
	"fmt"
)

// K8sClusterOverview holds aggregated cluster state for the dashboard.
type K8sClusterOverview struct {
	NamespaceCount  int                    `json:"namespace_count"`
	NodeCount       int                    `json:"node_count"`
	NodeReadyCount  int                    `json:"node_ready_count"`
	PodStatus       K8sPodStatusBreakdown  `json:"pod_status"`
	WorkloadCount   int                    `json:"workload_count"`
	WorkloadHealthy int                    `json:"workload_healthy"`
	ClusterHealth   string                 `json:"cluster_health"` // healthy, degraded, unhealthy
	Namespaces      []K8sNamespaceSummary  `json:"namespaces"`
}

// K8sPodStatusBreakdown counts pods by phase.
type K8sPodStatusBreakdown struct {
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
	Unknown   int `json:"unknown"`
}

// K8sNamespaceSummary holds per-namespace workload and pod counts.
type K8sNamespaceSummary struct {
	Name          string `json:"name"`
	WorkloadCount int    `json:"workload_count"`
	PodCount      int    `json:"pod_count"`
	Healthy       bool   `json:"healthy"`
}

// ClusterOverview aggregates cluster-wide state from namespaces, nodes,
// workloads, and pods into a single overview structure.
func (r *Runtime) ClusterOverview(ctx context.Context) (*K8sClusterOverview, error) {
	namespaces, err := r.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster overview: list namespaces: %w", err)
	}

	nodes, err := r.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster overview: list nodes: %w", err)
	}

	groups, err := r.ListWorkloads(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cluster overview: list workloads: %w", err)
	}

	pods, err := r.ListPods(ctx, nil, PodFilters{})
	if err != nil {
		return nil, fmt.Errorf("cluster overview: list pods: %w", err)
	}

	// Pod status breakdown.
	var podStatus K8sPodStatusBreakdown
	// Per-namespace pod counts.
	nsPodCount := make(map[string]int, len(namespaces))
	for _, p := range pods {
		nsPodCount[p.Namespace]++
		switch p.Status {
		case "Running":
			podStatus.Running++
		case "Pending":
			podStatus.Pending++
		case "Failed":
			podStatus.Failed++
		case "Succeeded":
			podStatus.Succeeded++
		default:
			podStatus.Unknown++
		}
	}

	// Workload counts.
	totalWorkloads := 0
	healthyWorkloads := 0
	nsWorkloadCount := make(map[string]int, len(namespaces))
	nsWorkloadHealthy := make(map[string]int, len(namespaces))
	for _, g := range groups {
		for _, wl := range g.Workloads {
			totalWorkloads++
			nsWorkloadCount[wl.Namespace]++
			if wl.Status == "healthy" {
				healthyWorkloads++
				nsWorkloadHealthy[wl.Namespace]++
			}
		}
	}

	// Node counts.
	nodeReadyCount := 0
	anyNodeNotReady := false
	anyNodeWarning := false
	for _, n := range nodes {
		if n.Status == "ready" {
			nodeReadyCount++
		} else if n.Status == "not-ready" {
			anyNodeNotReady = true
		}
		// Check for pressure conditions.
		for _, c := range n.Conditions {
			if c.Status == "True" && (c.Type == "MemoryPressure" || c.Type == "DiskPressure" || c.Type == "PIDPressure") {
				anyNodeWarning = true
			}
		}
	}

	// Per-namespace summaries.
	nsSummaries := make([]K8sNamespaceSummary, 0, len(namespaces))
	for _, ns := range namespaces {
		wlCount := nsWorkloadCount[ns]
		wlHealthy := nsWorkloadHealthy[ns]
		nsSummaries = append(nsSummaries, K8sNamespaceSummary{
			Name:          ns,
			WorkloadCount: wlCount,
			PodCount:      nsPodCount[ns],
			Healthy:       wlCount == 0 || wlCount == wlHealthy,
		})
	}

	// Derive cluster health.
	clusterHealth := deriveClusterHealth(anyNodeNotReady, anyNodeWarning, totalWorkloads, healthyWorkloads, groups)

	return &K8sClusterOverview{
		NamespaceCount:  len(namespaces),
		NodeCount:       len(nodes),
		NodeReadyCount:  nodeReadyCount,
		PodStatus:       podStatus,
		WorkloadCount:   totalWorkloads,
		WorkloadHealthy: healthyWorkloads,
		ClusterHealth:   clusterHealth,
		Namespaces:      nsSummaries,
	}, nil
}

// deriveClusterHealth computes cluster-level health string.
//   - healthy: all nodes Ready, no pressure conditions, all workloads healthy
//   - degraded: any node pressure condition OR any workload under-replicated
//   - unhealthy: any node NotReady OR any workload with zero ready replicas
func deriveClusterHealth(anyNodeNotReady, anyNodeWarning bool, totalWorkloads, healthyWorkloads int, groups []K8sWorkloadGroup) string {
	// Check for unhealthy: node not-ready or any workload with zero ready + desired > 0.
	if anyNodeNotReady {
		return "unhealthy"
	}
	for _, g := range groups {
		for _, wl := range g.Workloads {
			if wl.DesiredReplicas > 0 && wl.ReadyReplicas == 0 {
				return "unhealthy"
			}
		}
	}

	// Check for degraded: node warning or any under-replicated workload.
	if anyNodeWarning {
		return "degraded"
	}
	if totalWorkloads > 0 && healthyWorkloads < totalWorkloads {
		return "degraded"
	}

	return "healthy"
}
