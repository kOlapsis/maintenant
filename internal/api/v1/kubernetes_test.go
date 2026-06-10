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

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
)

type fakeK8sStore struct {
	lastAgentID string
	workloads   []kubernetes.K8sWorkloadGroup
	pods        []kubernetes.K8sPod
	nodes       []kubernetes.K8sNode
}

func (f *fakeK8sStore) ListNamespaces(_ context.Context, agentID string) ([]string, error) {
	f.lastAgentID = agentID
	return []string{"default"}, nil
}
func (f *fakeK8sStore) ListWorkloads(_ context.Context, agentID string, _ []string) ([]kubernetes.K8sWorkloadGroup, error) {
	f.lastAgentID = agentID
	return f.workloads, nil
}
func (f *fakeK8sStore) GetWorkload(_ context.Context, agentID, _ string) (*kubernetes.K8sWorkload, error) {
	f.lastAgentID = agentID
	return nil, nil
}
func (f *fakeK8sStore) ListPods(_ context.Context, agentID string, _ []string, _ kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	f.lastAgentID = agentID
	return f.pods, nil
}
func (f *fakeK8sStore) GetPod(_ context.Context, agentID, _, _ string) (*kubernetes.K8sPod, error) {
	f.lastAgentID = agentID
	return nil, nil
}
func (f *fakeK8sStore) ListNodes(_ context.Context, agentID string) ([]kubernetes.K8sNode, error) {
	f.lastAgentID = agentID
	return f.nodes, nil
}
func (f *fakeK8sStore) ListEventsForObject(_ context.Context, agentID, _, _, _ string) ([]kubernetes.K8sEvent, error) {
	f.lastAgentID = agentID
	return nil, nil
}

func TestKubernetesHandler_ListWorkloads_ResolvesAgentAndShape(t *testing.T) {
	store := &fakeK8sStore{
		workloads: []kubernetes.K8sWorkloadGroup{
			{Namespace: "default", Workloads: []kubernetes.K8sWorkload{
				{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Status: "healthy", ReadyReplicas: 3, DesiredReplicas: 3},
			}},
		},
	}
	h := NewKubernetesHandler(store, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/kubernetes/workloads", h.HandleListWorkloads)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/workloads?agent_id=local", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uid.LocalAgent, store.lastAgentID, "agent_id=local must resolve to the LocalAgent sentinel")

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total"])
	groups := body["groups"].([]interface{})
	require.Len(t, groups, 1)
}

func TestKubernetesHandler_ListPods_RemoteAgentVerbatim(t *testing.T) {
	store := &fakeK8sStore{}
	h := NewKubernetesHandler(store, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/kubernetes/pods", h.HandleListPods)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/pods?agent_id=agent-XYZ", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "agent-XYZ", store.lastAgentID, "remote agent id passed through verbatim")
}

type stubK8sAgentDir struct{ names map[string]AgentName }

func (s stubK8sAgentDir) AgentNames(context.Context) (map[string]AgentName, error) {
	return s.names, nil
}

func TestKubernetesHandler_ListWorkloads_EnrichesRemoteAgent(t *testing.T) {
	store := &fakeK8sStore{
		workloads: []kubernetes.K8sWorkloadGroup{
			{Namespace: "default", Workloads: []kubernetes.K8sWorkload{
				{ID: "default/Deployment/web", AgentID: "agent-XYZ", Name: "web", Namespace: "default", Kind: "Deployment", Status: "healthy"},
				{ID: "default/Deployment/api", AgentID: uid.LocalAgent, Name: "api", Namespace: "default", Kind: "Deployment", Status: "healthy"},
			}},
		},
	}
	h := NewKubernetesHandler(store, nil)
	h.SetAgentDirectory(stubK8sAgentDir{names: map[string]AgentName{
		"agent-XYZ": {Hostname: "edge-1", Label: "Edge cluster"},
	}})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/kubernetes/workloads", h.HandleListWorkloads)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/workloads", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	wls := body["groups"].([]interface{})[0].(map[string]interface{})["workloads"].([]interface{})

	byName := map[string]map[string]interface{}{}
	for _, raw := range wls {
		m := raw.(map[string]interface{})
		byName[m["name"].(string)] = m
	}

	remote := byName["web"] // reported by a remote agent
	assert.Equal(t, "agent-XYZ", remote["agent_id"])
	assert.Equal(t, "Edge cluster", remote["agent_label"])
	assert.Equal(t, "edge-1", remote["agent_hostname"])

	_, hasAgent := byName["api"]["agent_id"] // LocalAgent → no agent badge
	assert.False(t, hasAgent, "local rows carry no agent badge")
}

func TestKubernetesHandler_Cluster_AggregatesFromStore(t *testing.T) {
	store := &fakeK8sStore{
		workloads: []kubernetes.K8sWorkloadGroup{
			{Namespace: "default", Workloads: []kubernetes.K8sWorkload{
				{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Status: "healthy"},
				{ID: "default/Deployment/api", Name: "api", Namespace: "default", Kind: "Deployment", Status: "degraded"},
			}},
		},
		pods: []kubernetes.K8sPod{
			{Namespace: "default", Name: "web-1", Status: "Running"},
			{Namespace: "default", Name: "api-1", Status: "Pending"},
		},
		nodes: []kubernetes.K8sNode{
			{Name: "n1", Status: "ready"},
		},
	}
	h := NewKubernetesHandler(store, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/kubernetes/cluster", h.HandleGetCluster)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/cluster", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(2), body["workload_count"])
	assert.Equal(t, float64(1), body["workload_healthy"])
	assert.Equal(t, float64(1), body["node_count"])
	assert.Equal(t, float64(1), body["node_ready_count"])
	assert.Equal(t, "degraded", body["cluster_health"])
	podStatus := body["pod_status"].(map[string]interface{})
	assert.Equal(t, float64(1), podStatus["running"])
	assert.Equal(t, float64(1), podStatus["pending"])
}
