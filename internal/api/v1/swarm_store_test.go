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

type fakeSwarmTopo struct {
	lastAgentID   string
	lastServiceID string
	services      []*swarm.SwarmService
	tasks         []*swarm.SwarmTask
}

func (f *fakeSwarmTopo) ListServices(_ context.Context, agentID string) ([]*swarm.SwarmService, error) {
	f.lastAgentID = agentID
	return f.services, nil
}
func (f *fakeSwarmTopo) ListTasks(_ context.Context, agentID, serviceID string) ([]*swarm.SwarmTask, error) {
	f.lastAgentID = agentID
	f.lastServiceID = serviceID
	return f.tasks, nil
}

func newSwarmHandlerForTest(topo swarmTopologyReader) *SwarmHandler {
	return NewSwarmHandler(
		func() *swarm.SwarmCluster { return nil },
		func() *swarm.ServiceDiscovery { return nil },
		func() *swarm.Detector { return nil },
		topo, nil, nil, nil, nil, nil, nil,
	)
}

func TestSwarmHandler_ListServices_StoreBackedAndAgentScoped(t *testing.T) {
	topo := &fakeSwarmTopo{
		services: []*swarm.SwarmService{
			{ServiceID: "svc1", Name: "web", Image: "nginx", Mode: "replicated", DesiredReplicas: 3, RunningReplicas: 3},
		},
	}
	h := newSwarmHandlerForTest(topo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/swarm/services", h.HandleListServices)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swarm/services?agent_id=local", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uid.LocalAgent, topo.lastAgentID, "agent_id=local resolves to the LocalAgent sentinel")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total"])
}

func TestSwarmHandler_ListTasks_EnrichesServiceNameAndFilters(t *testing.T) {
	topo := &fakeSwarmTopo{
		services: []*swarm.SwarmService{{ServiceID: "svc1", Name: "web"}},
		tasks: []*swarm.SwarmTask{
			{TaskID: "t1", ServiceID: "svc1", NodeID: "n1", State: "running"},
			{TaskID: "t2", ServiceID: "svc1", NodeID: "n2", State: "failed"},
		},
	}
	h := newSwarmHandlerForTest(topo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/swarm/tasks", h.HandleListTasks)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/swarm/tasks?agent_id=agent-XYZ&node=n1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "agent-XYZ", topo.lastAgentID)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	tasks := body["tasks"].([]interface{})
	require.Len(t, tasks, 1, "node=n1 filter keeps a single task")
	task := tasks[0].(map[string]interface{})
	assert.Equal(t, "web", task["service_name"], "task is enriched with its service name")
}
