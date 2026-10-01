// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	dockerswarm "github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/swarm"
)

type liveSwarmClient struct{ svc dockerswarm.Service }

func (c liveSwarmClient) ServiceList(context.Context) ([]dockerswarm.Service, error) {
	return []dockerswarm.Service{c.svc}, nil
}
func (c liveSwarmClient) ServiceInspect(context.Context, string) (dockerswarm.Service, error) {
	return c.svc, nil
}
func (c liveSwarmClient) TaskList(context.Context) ([]dockerswarm.Task, error) { return nil, nil }
func (c liveSwarmClient) NodeList(context.Context) ([]dockerswarm.Node, error) { return nil, nil }

func TestSwarmHandler_ReadsTheUpdateTrackerOfTheMomentNotOfConstruction(t *testing.T) {
	client := liveSwarmClient{svc: dockerswarm.Service{
		ID:           "svc1",
		Spec:         dockerswarm.ServiceSpec{Annotations: dockerswarm.Annotations{Name: "prod_web"}},
		UpdateStatus: &dockerswarm.UpdateStatus{State: dockerswarm.UpdateStateUpdating},
	}}
	disc := swarm.NewServiceDiscovery(client, slog.Default())
	_, err := disc.DiscoverAll(context.Background())
	require.NoError(t, err)

	var tracker atomic.Pointer[swarm.UpdateTracker]
	h := NewSwarmHandler(
		func() *swarm.SwarmCluster { return nil },
		func() *swarm.ServiceDiscovery { return disc },
		func() *swarm.Detector { return nil },
		&fakeSwarmTopo{}, nil,
		tracker.Load,
		func() *swarm.CrashLoopDetector { return nil },
		nil, nil,
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/swarm/services/{serviceID}/update-status", h.HandleGetUpdateStatus)

	progress := func() any {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/swarm/services/svc1/update-status", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body["progress"]
	}

	assert.Nil(t, progress())
	tracker.Store(swarm.NewUpdateTracker(client, slog.Default()))
	assert.NotNil(t, progress(), "a tracker built after the handler is used")
}

func TestSwarmDashboard_ServesOnlyWhatItHasASourceFor(t *testing.T) {
	h := NewSwarmHandler(
		func() *swarm.SwarmCluster { return &swarm.SwarmCluster{ID: "cluster-1", IsManager: true} },
		func() *swarm.ServiceDiscovery { return nil },
		func() *swarm.Detector { return nil },
		&fakeSwarmTopo{}, nil,
		func() *swarm.UpdateTracker { return nil },
		func() *swarm.CrashLoopDetector { return nil },
		nil, nil,
	)
	w := httptest.NewRecorder()
	h.HandleGetDashboard(w, httptest.NewRequest(http.MethodGet, "/api/v1/swarm/dashboard", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.ElementsMatch(t, []string{"cluster", "nodes", "services"}, slices.Collect(maps.Keys(body)))
}
