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
	"log/slog"
)

// TaskPlacement represents the distribution of tasks across nodes for a service.
type TaskPlacement struct {
	ServiceID   string                  `json:"service_id"`
	ServiceName string                  `json:"service_name"`
	ByNode      map[string][]SwarmTask  `json:"by_node"` // keyed by node ID
}

// TaskTracker aggregates task placement information from the Docker API.
type TaskTracker struct {
	client ServiceClient
	logger *slog.Logger
}

// NewTaskTracker creates a new task placement tracker.
func NewTaskTracker(client ServiceClient, logger *slog.Logger) *TaskTracker {
	return &TaskTracker{
		client: client,
		logger: logger,
	}
}

// GetPlacementForService returns the task distribution across nodes for a given service.
func (tt *TaskTracker) GetPlacementForService(ctx context.Context, serviceID, serviceName string) (*TaskPlacement, error) {
	tasks, err := tt.client.TaskList(ctx)
	if err != nil {
		return nil, err
	}

	nodes, err := tt.client.NodeList(ctx)
	if err != nil {
		tt.logger.Warn("failed to list nodes for hostname resolution", "error", err)
	}
	hostnames := make(map[string]string, len(nodes))
	for _, n := range nodes {
		hostnames[n.ID] = n.Description.Hostname
	}

	placement := &TaskPlacement{
		ServiceID:   serviceID,
		ServiceName: serviceName,
		ByNode:      make(map[string][]SwarmTask),
	}

	for _, t := range tasks {
		if t.ServiceID != serviceID {
			continue
		}
		st := SwarmTask{
			TaskID:       t.ID,
			ServiceID:    t.ServiceID,
			NodeID:       t.NodeID,
			Slot:         t.Slot,
			State:        string(t.Status.State),
			DesiredState: string(t.DesiredState),
			Timestamp:    t.Status.Timestamp,
			NodeHostname: hostnames[t.NodeID],
		}
		if t.Status.ContainerStatus != nil {
			st.ContainerID = t.Status.ContainerStatus.ContainerID
			if t.Status.ContainerStatus.ExitCode != 0 {
				code := t.Status.ContainerStatus.ExitCode
				st.ExitCode = &code
			}
		}
		if t.Status.Err != "" {
			st.Error = t.Status.Err
		}
		placement.ByNode[t.NodeID] = append(placement.ByNode[t.NodeID], st)
	}

	return placement, nil
}
