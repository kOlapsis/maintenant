// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"fmt"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	dockerswarm "github.com/moby/moby/api/types/swarm"
)

// SnapshotFromClient builds a full topology snapshot from the live swarm API.
// Shared by the agent collector (which marshals it to proto and pushes it) and
// the server's own local-runtime reconcile (which feeds it straight to the
// ingest service under the LocalAgent id). disc supplies services and tasks;
// client supplies nodes.
func SnapshotFromClient(ctx context.Context, disc *ServiceDiscovery, client ServiceClient) (TopologySnapshot, error) {
	services, tasks, err := disc.discover(ctx)
	if err != nil {
		return TopologySnapshot{}, err
	}
	nodes, err := client.NodeList(ctx)
	if err != nil && !cerrdefs.IsPermissionDenied(err) {
		return TopologySnapshot{}, fmt.Errorf("list nodes: %w", err)
	}

	tasksByService := make(map[string][]SwarmTask, len(services))
	taskCounts := make(map[string]int)
	for _, t := range tasks {
		if t.Status.State == dockerswarm.TaskStateRunning {
			taskCounts[t.NodeID]++
		}
		if isReportedTask(t) {
			tasksByService[t.ServiceID] = append(tasksByService[t.ServiceID], *mapTask(t))
		}
	}

	var snap TopologySnapshot
	for _, svc := range services {
		snap.Services = append(snap.Services, *svc)
		snap.Tasks = append(snap.Tasks, tasksByService[svc.ServiceID]...)
	}
	now := time.Now()
	for _, n := range nodes {
		snap.Nodes = append(snap.Nodes, mapDockerNode(n, taskCounts[n.ID], now))
	}

	return snap, nil
}

// mapDockerNode maps a docker SDK swarm node to the domain SwarmNode. Timestamps
// are stamped at snapshot time; the store preserves first_seen_at across upserts.
func mapDockerNode(n dockerswarm.Node, taskCount int, now time.Time) SwarmNode {
	return SwarmNode{
		NodeID:             n.ID,
		Hostname:           n.Description.Hostname,
		Role:               string(n.Spec.Role),
		Status:             string(n.Status.State),
		Availability:       string(n.Spec.Availability),
		EngineVersion:      n.Description.Engine.EngineVersion,
		Address:            n.Status.Addr,
		TaskCount:          taskCount,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		LastStatusChangeAt: now,
	}
}
