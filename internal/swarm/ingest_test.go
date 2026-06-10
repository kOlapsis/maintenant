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
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

type captureStore struct {
	agentID  string
	services []SwarmService
	tasks    []SwarmTask
	nodes    []*SwarmNode
}

func (c *captureStore) ReplaceServicesForAgent(_ context.Context, agentID string, services []SwarmService) error {
	c.agentID = agentID
	c.services = services
	return nil
}

func (c *captureStore) ReplaceTasksForAgent(_ context.Context, _ string, tasks []SwarmTask) error {
	c.tasks = tasks
	return nil
}

func (c *captureStore) ReplaceNodesForAgent(_ context.Context, _ string, nodes []*SwarmNode) error {
	c.nodes = nodes
	return nil
}

func TestIngestService_MapsSnapshot(t *testing.T) {
	cap := &captureStore{}
	svc := NewIngestService(cap, cap, nil)

	ev := &agentpb.SwarmTopology{
		Services: []*agentpb.SwarmServiceMsg{
			{ServiceId: "svc1", Name: "web", Image: "nginx", Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 2, StackName: "stk", Labels: map[string]string{"k": "v"}, CreatedAt: timestamppb.Now()},
		},
		Tasks: []*agentpb.SwarmTaskMsg{
			{TaskId: "t1", ServiceId: "svc1", Slot: 1, State: "running", DesiredState: "running"},
			{TaskId: "t2", ServiceId: "svc1", Slot: 2, State: "failed", ExitCode: 137, HasExitCode: true},
		},
		Nodes: []*agentpb.SwarmNodeMsg{
			{NodeId: "n1", Hostname: "mgr", Role: "manager", Status: "ready", Availability: "active", TaskCount: 5},
			{NodeId: "n2", Hostname: "bad", Role: "bogus", Status: "weird", Availability: "nope"},
		},
	}

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-A", ev))

	require.Equal(t, "agent-A", cap.agentID)

	require.Len(t, cap.services, 1)
	require.Equal(t, "web", cap.services[0].Name)
	require.Equal(t, 2, cap.services[0].RunningReplicas)
	require.Equal(t, "v", cap.services[0].Labels["k"])

	require.Len(t, cap.tasks, 2)
	require.Nil(t, cap.tasks[0].ExitCode, "no exit code when has_exit_code is false")
	require.NotNil(t, cap.tasks[1].ExitCode)
	require.Equal(t, 137, *cap.tasks[1].ExitCode)

	require.Len(t, cap.nodes, 2)
	require.Equal(t, "manager", cap.nodes[0].Role)
	// Invalid values are coerced to schema-valid defaults.
	require.Equal(t, "worker", cap.nodes[1].Role)
	require.Equal(t, "unknown", cap.nodes[1].Status)
	require.Equal(t, "active", cap.nodes[1].Availability)
}

func TestIngestService_NilEventIsNoop(t *testing.T) {
	cap := &captureStore{}
	svc := NewIngestService(cap, cap, nil)
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-A", nil))
	require.Nil(t, cap.services)
}

func TestIngestService_BroadcastsTopologyChanged(t *testing.T) {
	cap := &captureStore{}
	svc := NewIngestService(cap, cap, nil)

	var gotType, gotAgent string
	svc.SetBroadcaster(func(eventType string, data any) {
		gotType = eventType
		if m, ok := data.(map[string]any); ok {
			gotAgent, _ = m["agent_id"].(string)
		}
	})

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-A", &agentpb.SwarmTopology{}))
	require.Equal(t, "swarm.topology_changed", gotType)
	require.Equal(t, "agent-A", gotAgent)
}
