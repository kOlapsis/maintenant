// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestKubernetesWorkload_ListsOnlyItsOwnPods(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	db := storetest.Open(t, logger)
	require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
		AgentID: "kube-b", Hostname: "kube-b", Label: "kube-b", Status: "active",
		DetectedRuntime: "kubernetes", CreatedAt: time.Now(),
	}))
	k8s := store.NewKubernetesStore(db)
	web := kubernetes.K8sWorkload{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Status: "healthy"}
	require.NoError(t, k8s.ReplaceWorkloadsForAgent(ctx, "", []kubernetes.K8sWorkload{web}))
	require.NoError(t, k8s.ReplaceWorkloadsForAgent(ctx, "kube-b", []kubernetes.K8sWorkload{web}))
	require.NoError(t, k8s.ReplacePodsForAgent(ctx, "", []kubernetes.K8sPod{
		{Namespace: "default", Name: "web-1", Status: "Running", WorkloadRef: "default/Deployment/web"},
		{Namespace: "default", Name: "web-api-1", Status: "Running", WorkloadRef: "default/Deployment/web-api"},
	}))
	require.NoError(t, k8s.ReplacePodsForAgent(ctx, "kube-b", []kubernetes.K8sPod{
		{Namespace: "default", Name: "web-far-1", Status: "Running", WorkloadRef: "default/Deployment/web"},
	}))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/kubernetes/workloads/{id}", NewKubernetesHandler(k8s, nil).HandleGetWorkload)
	names := func(path string) []string {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Pods []struct {
				Name string `json:"name"`
			} `json:"pods"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		out := make([]string, 0, len(body.Pods))
		for _, p := range body.Pods {
			out = append(out, p.Name)
		}
		return out
	}

	assert.Equal(t, []string{"web-1"}, names("/api/v1/kubernetes/workloads/default%2FDeployment%2Fweb"))
	assert.Equal(t, []string{"web-far-1"}, names("/api/v1/kubernetes/workloads/default%2FDeployment%2Fweb?agent_id=kube-b"))
}
