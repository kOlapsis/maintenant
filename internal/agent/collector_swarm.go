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

package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/swarm"
)

const swarmTopologyInterval = 30 * time.Second

// streamSwarmTopology periodically snapshots the agent's swarm (services, tasks,
// nodes) and pushes a full SwarmTopology event. The server reconciles each
// snapshot against the rows it holds for this agent, hard-deleting whatever is
// gone. Sends one snapshot immediately so the server has state before the first
// tick. Blocks until ctx is cancelled or a push fails.
func streamSwarmTopology(ctx context.Context, id *Identity, disc *swarm.ServiceDiscovery, client swarm.ServiceClient, stream *PushStream, logger *slog.Logger) error {
	send := func() error {
		snap, err := swarm.SnapshotFromClient(ctx, disc, client)
		if err != nil {
			logger.Warn("collector: swarm topology snapshot failed", "err", err)
			return nil
		}
		return stream.Send(swarmTopologyEvent(id.AgentID, snap))
	}

	if err := send(); err != nil {
		return err
	}

	ticker := time.NewTicker(swarmTopologyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := send(); err != nil {
				return err
			}
		}
	}
}

// swarmTopologyEvent marshals a domain snapshot into an AgentEvent. Pure, so it
// is unit-tested without a live runtime or stream.
func swarmTopologyEvent(agentID string, snap swarm.TopologySnapshot) *agentpb.AgentEvent {
	services := make([]*agentpb.SwarmServiceMsg, 0, len(snap.Services))
	for i := range snap.Services {
		s := &snap.Services[i]
		services = append(services, &agentpb.SwarmServiceMsg{
			ServiceId:       s.ServiceID,
			Name:            s.Name,
			Image:           s.Image,
			Mode:            s.Mode,
			DesiredReplicas: clampInt32(s.DesiredReplicas),
			RunningReplicas: clampInt32(s.RunningReplicas),
			Labels:          s.Labels,
			StackName:       s.StackName,
			CreatedAt:       timestamppb.New(s.CreatedAt),
		})
	}

	tasks := make([]*agentpb.SwarmTaskMsg, 0, len(snap.Tasks))
	for i := range snap.Tasks {
		t := &snap.Tasks[i]
		msg := &agentpb.SwarmTaskMsg{
			TaskId:       t.TaskID,
			ServiceId:    t.ServiceID,
			NodeId:       t.NodeID,
			Slot:         clampInt32(t.Slot),
			State:        t.State,
			DesiredState: t.DesiredState,
			ContainerId:  t.ContainerID,
			Error:        t.Error,
			Timestamp:    timestamppb.New(t.Timestamp),
			NodeHostname: t.NodeHostname,
		}
		if t.ExitCode != nil {
			msg.ExitCode = clampInt32(*t.ExitCode)
			msg.HasExitCode = true
		}
		tasks = append(tasks, msg)
	}

	nodes := make([]*agentpb.SwarmNodeMsg, 0, len(snap.Nodes))
	for i := range snap.Nodes {
		n := &snap.Nodes[i]
		nodes = append(nodes, &agentpb.SwarmNodeMsg{
			NodeId:        n.NodeID,
			Hostname:      n.Hostname,
			Role:          n.Role,
			Status:        n.Status,
			Availability:  n.Availability,
			EngineVersion: n.EngineVersion,
			Address:       n.Address,
			TaskCount:     clampInt32(n.TaskCount),
		})
	}

	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Swarm{Swarm: &agentpb.SwarmTopology{
			Services: services,
			Tasks:    tasks,
			Nodes:    nodes,
		}},
	}
}
