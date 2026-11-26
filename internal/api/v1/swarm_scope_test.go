// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeSwarmNodes struct {
	swarm.NodeStore
	nodes []*swarm.SwarmNode
}

func (f fakeSwarmNodes) ListNodes(_ context.Context, agentID string) ([]*swarm.SwarmNode, error) {
	var out []*swarm.SwarmNode
	for _, n := range f.nodes {
		if agentID == "" || n.AgentID == agentID {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f fakeSwarmNodes) GetNodeByNodeID(_ context.Context, nodeID string) (*swarm.SwarmNode, error) {
	for _, n := range f.nodes {
		if n.NodeID == nodeID {
			return n, nil
		}
	}
	return nil, nil
}

func scopedSwarmHandler(topo swarmTopologyReader) *SwarmHandler {
	return NewSwarmHandler(
		func() *swarm.SwarmCluster { return &swarm.SwarmCluster{ID: "cluster-1", IsManager: true} },
		func() *swarm.ServiceDiscovery { return nil },
		func() *swarm.Detector { return nil },
		topo,
		fakeSwarmNodes{nodes: []*swarm.SwarmNode{
			{AgentID: uid.LocalAgent, NodeID: "local-1", Hostname: "mgr", Status: "ready", Availability: "active"},
			{AgentID: "agent-b", NodeID: "remote-1", Hostname: "far", Status: "down", Availability: "active", TaskCount: 1},
		}},
		func() *swarm.UpdateTracker { return nil },
		func() *swarm.CrashLoopDetector { return nil },
		nil, nil,
	)
}

func swarmGet(t *testing.T, h http.HandlerFunc, pattern, path string) map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, h)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestSwarmCluster_IgnoresTheNodesOfRemoteAgents(t *testing.T) {
	h := scopedSwarmHandler(&fakeSwarmTopo{})

	body := swarmGet(t, h.HandleGetCluster, "GET /api/v1/swarm/cluster", "/api/v1/swarm/cluster")

	assert.Equal(t, swarm.ClusterHealthHealthy, body["cluster_health"])
	assert.Equal(t, map[string]any{"ready": float64(1), "down": float64(0), "disconnected": float64(0)}, body["nodes"])
}

func TestSwarmDashboard_ListsOnlyTheLocalNodes(t *testing.T) {
	h := scopedSwarmHandler(&fakeSwarmTopo{})

	body := swarmGet(t, h.HandleGetDashboard, "GET /api/v1/swarm/dashboard", "/api/v1/swarm/dashboard")

	nodes := body["nodes"].([]any)
	require.Len(t, nodes, 1)
	assert.Equal(t, "local-1", nodes[0].(map[string]any)["node_id"])
}

func TestSwarmNodeDetail_RemoteNodeListsItsStoredTasks(t *testing.T) {
	topo := &fakeSwarmTopo{
		services: []*swarm.SwarmService{{ServiceID: "svc1", Name: "web", Image: "nginx:1.27"}},
		tasks: []*swarm.SwarmTask{
			{TaskID: "t1", ServiceID: "svc1", NodeID: "remote-1", Slot: 1, State: "running"},
			{TaskID: "t2", ServiceID: "svc1", NodeID: "remote-2", Slot: 2, State: "running"},
		},
	}
	h := scopedSwarmHandler(topo)

	body := swarmGet(t, h.HandleGetNodeDetail, "GET /api/v1/swarm/nodes/{nodeID}", "/api/v1/swarm/nodes/remote-1")

	assert.Equal(t, "agent-b", topo.lastAgentID)
	tasks := body["tasks"].([]any)
	require.Len(t, tasks, 1)
	task := tasks[0].(map[string]any)
	assert.Equal(t, "t1", task["task_id"])
	assert.Equal(t, "web", task["service_name"])
	assert.Equal(t, "nginx:1.27", task["image"])
}
