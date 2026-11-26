// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	dockerswarm "github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/swarm"
)

func TestSwarmTopologyEvent_MarshalsSnapshot(t *testing.T) {
	exit := 137
	snap := swarm.TopologySnapshot{
		Services: []swarm.SwarmService{
			{ServiceID: "svc1", Name: "web", Image: "nginx", Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 2, StackName: "stk", Labels: map[string]string{"k": "v"}},
		},
		Tasks: []swarm.SwarmTask{
			{TaskID: "t1", ServiceID: "svc1", Slot: 1, State: "running"},
			{TaskID: "t2", ServiceID: "svc1", Slot: 2, State: "failed", ExitCode: &exit},
		},
		Nodes: []swarm.SwarmNode{
			{NodeID: "n1", Hostname: "mgr", Role: "manager", Status: "ready", Availability: "active", TaskCount: 5},
		},
	}

	ev := swarmTopologyEvent("agent-A", snap)

	require.Equal(t, "agent-A", ev.GetAgentId())
	require.NotEmpty(t, ev.GetEventId())

	body := ev.GetSwarm()
	require.NotNil(t, body, "event body must be a swarm topology")

	require.Len(t, body.GetServices(), 1)
	require.Equal(t, "web", body.GetServices()[0].GetName())
	require.Equal(t, int32(2), body.GetServices()[0].GetRunningReplicas())
	require.Equal(t, "v", body.GetServices()[0].GetLabels()["k"])

	require.Len(t, body.GetTasks(), 2)
	require.False(t, body.GetTasks()[0].GetHasExitCode())
	require.True(t, body.GetTasks()[1].GetHasExitCode())
	require.Equal(t, int32(137), body.GetTasks()[1].GetExitCode())

	require.Len(t, body.GetNodes(), 1)
	require.Equal(t, "manager", body.GetNodes()[0].GetRole())
	require.Equal(t, int32(5), body.GetNodes()[0].GetTaskCount())
}

type failingSwarmClient struct{ down atomic.Bool }

func (c *failingSwarmClient) ServiceList(context.Context) ([]dockerswarm.Service, error) {
	if c.down.Load() {
		return nil, errors.New("the swarm does not have a leader")
	}
	return nil, nil
}

func (c *failingSwarmClient) ServiceInspect(context.Context, string) (dockerswarm.Service, error) {
	return dockerswarm.Service{}, nil
}
func (c *failingSwarmClient) TaskList(context.Context) ([]dockerswarm.Task, error) { return nil, nil }
func (c *failingSwarmClient) NodeList(context.Context) ([]dockerswarm.Node, error) { return nil, nil }

func TestStreamSwarmTopology_ReportsAManagerThatStopsAnswering(t *testing.T) {
	prev := swarmTopologyInterval
	swarmTopologyInterval = 2 * time.Millisecond
	t.Cleanup(func() { swarmTopologyInterval = prev })

	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })

	client := &failingSwarmClient{}
	client.down.Store(true)
	id := &Identity{AgentID: "agent-1"}
	watch := &runtimeWatch{agentID: id.AgentID, label: RuntimeSwarm, spool: spool, logger: testLogger()}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- streamSwarmTopology(ctx, id, swarm.NewServiceDiscovery(client, testLogger()), client, watch, spool, testLogger())
	}()

	last := func() (runtimeReport, bool) {
		reports := runtimeReports(sink)
		if len(reports) == 0 {
			return runtimeReport{}, false
		}
		return reports[len(reports)-1], true
	}
	swarmKind := agentpb.Runtime_RUNTIME_SWARM
	require.Eventually(t, func() bool {
		r, ok := last()
		return ok && r == runtimeReport{swarmKind, true}
	}, 5*time.Second, 2*time.Millisecond, "failed snapshots must end up reported")

	client.down.Store(false)
	require.Eventually(t, func() bool {
		r, ok := last()
		return ok && r == runtimeReport{swarmKind, false}
	}, 5*time.Second, 2*time.Millisecond, "the first snapshot that works again must be reported")

	cancel()
	require.NoError(t, <-done)
}
