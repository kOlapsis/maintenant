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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	dockerswarm "github.com/docker/docker/api/types/swarm"
	dockersystem "github.com/docker/docker/api/types/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/swarm"
)

// fakeInfoProvider feeds the Swarm detector a canned docker info response.
type fakeInfoProvider struct {
	state   dockerswarm.LocalNodeState
	manager bool
}

func (f fakeInfoProvider) Info(context.Context) (dockersystem.Info, error) {
	return dockersystem.Info{Swarm: dockerswarm.Info{
		LocalNodeState:   f.state,
		ControlAvailable: f.manager,
	}}, nil
}

// fakeServiceClient feeds the discovery a fixed number of services.
type fakeServiceClient struct{ services []dockerswarm.Service }

func (f fakeServiceClient) ServiceList(context.Context) ([]dockerswarm.Service, error) {
	return f.services, nil
}
func (f fakeServiceClient) ServiceInspect(context.Context, string) (dockerswarm.Service, error) {
	return dockerswarm.Service{}, nil
}
func (f fakeServiceClient) TaskList(context.Context) ([]dockerswarm.Task, error) { return nil, nil }
func (f fakeServiceClient) NodeList(context.Context) ([]dockerswarm.Node, error) { return nil, nil }

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// activeManagerDetector returns a Swarm detector whose cached result reports an
// active manager node.
func activeManagerDetector(t *testing.T) *swarm.Detector {
	t.Helper()
	d := swarm.NewDetector(fakeInfoProvider{state: "active", manager: true}, testLogger())
	_, err := d.Detect(context.Background())
	require.NoError(t, err)
	return d
}

// discoveryWithServices returns a discovery whose cache holds n services.
func discoveryWithServices(t *testing.T, n int) *swarm.ServiceDiscovery {
	t.Helper()
	svcs := make([]dockerswarm.Service, n)
	for i := range svcs {
		svcs[i] = dockerswarm.Service{ID: "svc" + string(rune('a'+i))}
	}
	disc := swarm.NewServiceDiscovery(fakeServiceClient{services: svcs}, testLogger())
	_, _, err := disc.DiscoverAll(context.Background())
	require.NoError(t, err)
	return disc
}

func runtimeStatusBody(t *testing.T, d HandlerDeps) map[string]interface{} {
	t.Helper()
	r := &Router{mux: http.NewServeMux()}
	r.mux.HandleFunc("GET /api/v1/runtime/status", r.handleRuntimeStatus(d))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/status", nil)
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestRuntimeStatus_SwarmManagerEmpty_ServiceCountZero(t *testing.T) {
	det := activeManagerDetector(t)
	disc := discoveryWithServices(t, 0)
	body := runtimeStatusBody(t, HandlerDeps{
		Runtime:        &mockRuntime{name: "docker", connected: true},
		SwarmDetector:  func() *swarm.Detector { return det },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return disc },
		SwarmCluster:   func() *swarm.SwarmCluster { return nil },
	})

	assert.Equal(t, "swarm", body["context"])
	meta := body["metadata"].(map[string]interface{})
	assert.Equal(t, float64(0), meta["service_count"])
}

func TestRuntimeStatus_SwarmManagerWithServices_CountsThem(t *testing.T) {
	det := activeManagerDetector(t)
	disc := discoveryWithServices(t, 2)
	body := runtimeStatusBody(t, HandlerDeps{
		Runtime:        &mockRuntime{name: "docker", connected: true},
		SwarmDetector:  func() *swarm.Detector { return det },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return disc },
		SwarmCluster:   func() *swarm.SwarmCluster { return nil },
	})

	assert.Equal(t, "swarm", body["context"])
	meta := body["metadata"].(map[string]interface{})
	assert.Equal(t, float64(2), meta["service_count"])
}

func TestRuntimeStatus_SwarmManager_NilDiscovery_NoPanic(t *testing.T) {
	det := activeManagerDetector(t)
	body := runtimeStatusBody(t, HandlerDeps{
		Runtime:        &mockRuntime{name: "docker", connected: true},
		SwarmDetector:  func() *swarm.Detector { return det },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return nil },
		SwarmCluster:   func() *swarm.SwarmCluster { return nil },
	})

	assert.Equal(t, "swarm", body["context"])
	meta := body["metadata"].(map[string]interface{})
	assert.Equal(t, float64(0), meta["service_count"])
}

func TestRuntimeStatus_PlainDocker_NoServiceCount(t *testing.T) {
	body := runtimeStatusBody(t, HandlerDeps{
		Runtime:        &mockRuntime{name: "docker", connected: true},
		SwarmDetector:  func() *swarm.Detector { return nil },
		SwarmDiscovery: func() *swarm.ServiceDiscovery { return nil },
		SwarmCluster:   func() *swarm.SwarmCluster { return nil },
	})

	assert.Equal(t, "docker", body["context"])
	meta := body["metadata"].(map[string]interface{})
	_, has := meta["service_count"]
	assert.False(t, has, "plain docker metadata must not carry service_count")
}
