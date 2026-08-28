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
	"testing"

	"github.com/stretchr/testify/require"

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
