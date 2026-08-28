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

import (
	"context"
	"time"

	dockerswarm "github.com/docker/docker/api/types/swarm"
)

// SnapshotFromClient builds a full topology snapshot from the live swarm API.
// Shared by the agent collector (which marshals it to proto and pushes it) and
// the server's own local-runtime reconcile (which feeds it straight to the
// ingest service under the LocalAgent id). disc supplies services and tasks;
// client supplies nodes.
func SnapshotFromClient(ctx context.Context, disc *ServiceDiscovery, client ServiceClient) (TopologySnapshot, error) {
	_, services, err := disc.DiscoverAll(ctx)
	if err != nil {
		return TopologySnapshot{}, err
	}

	var snap TopologySnapshot
	for _, svc := range services {
		snap.Services = append(snap.Services, *svc)
		for _, t := range disc.GetTasksForService(svc.ServiceID) {
			snap.Tasks = append(snap.Tasks, *t)
		}
	}

	nodes, err := client.NodeList(ctx)
	if err == nil {
		taskCounts := make(map[string]int)
		if tasks, terr := client.TaskList(ctx); terr == nil {
			for _, t := range tasks {
				if t.Status.State == "running" {
					taskCounts[t.NodeID]++
				}
			}
		}
		now := time.Now()
		for _, n := range nodes {
			snap.Nodes = append(snap.Nodes, mapDockerNode(n, taskCounts[n.ID], now))
		}
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
