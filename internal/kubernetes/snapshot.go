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

// TopologySnapshot is a full point-in-time view of an agent's Kubernetes
// cluster. The server reconciles it against the rows already held for that
// agent, hard-deleting anything absent from the snapshot. Produced by the
// server's local runtime (under the LocalAgent id) and by remote agents (under
// their own id).
type TopologySnapshot struct {
	Namespaces []string
	Workloads  []K8sWorkload
	Pods       []K8sPod
	Nodes      []K8sNode
	Events     []K8sEventRef
}

// SnapshotSource is the subset of the Kubernetes runtime needed to build a
// topology snapshot. *Runtime satisfies it.
type SnapshotSource interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListWorkloads(ctx context.Context, namespaces []string) ([]K8sWorkloadGroup, error)
	ListPods(ctx context.Context, namespaces []string, filters PodFilters) ([]K8sPod, error)
	ListNodes(ctx context.Context) ([]K8sNode, error)
	ListAllEvents(ctx context.Context) ([]K8sEventRef, error)
}

// SnapshotFromRuntime queries the live cluster for namespaces, workloads, pods
// and nodes. Nodes are best-effort: a cluster-scoped RBAC denial on nodes does
// not fail the whole snapshot (the agent may only be granted namespaced access).
func SnapshotFromRuntime(ctx context.Context, src SnapshotSource) (TopologySnapshot, error) {
	var snap TopologySnapshot

	namespaces, err := src.ListNamespaces(ctx)
	if err != nil {
		return TopologySnapshot{}, fmt.Errorf("list namespaces: %w", err)
	}
	snap.Namespaces = namespaces

	groups, err := src.ListWorkloads(ctx, nil)
	if err != nil {
		return TopologySnapshot{}, fmt.Errorf("list workloads: %w", err)
	}
	for _, g := range groups {
		snap.Workloads = append(snap.Workloads, g.Workloads...)
	}

	pods, err := src.ListPods(ctx, nil, PodFilters{})
	if err != nil {
		return TopologySnapshot{}, fmt.Errorf("list pods: %w", err)
	}
	snap.Pods = pods

	if nodes, nerr := src.ListNodes(ctx); nerr == nil {
		// A Node object carries no running-pod count; derive it from the pods
		// scheduled on each node (spec.nodeName).
		perNode := make(map[string]int, len(nodes))
		for i := range pods {
			if pods[i].NodeName != "" {
				perNode[pods[i].NodeName]++
			}
		}
		for i := range nodes {
			nodes[i].RunningPods = perNode[nodes[i].Name]
		}
		snap.Nodes = nodes
	}

	// Events are best-effort: a cluster-scoped RBAC denial does not fail the
	// whole snapshot.
	if events, eerr := src.ListAllEvents(ctx); eerr == nil {
		snap.Events = events
	}

	return snap, nil
}
