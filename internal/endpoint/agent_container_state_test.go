// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type agentEndpointEnv struct {
	t     *testing.T
	ctx   context.Context
	store *store.EndpointStore
	svc   *endpoint.Service
}

const stateAgent = "00000000-0000-0000-0000-0000000000e1"

func newAgentEndpointEnv(t *testing.T) *agentEndpointEnv {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	ctx := context.Background()
	require.NoError(t, store.NewAgentStore(db).Insert(ctx, &agent.Agent{
		AgentID: stateAgent, Hostname: "nas", Label: "nas", OSArch: "linux/amd64",
		AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
	}))
	e := &agentEndpointEnv{t: t, ctx: ctx, store: store.NewEndpointStore(db)}
	e.svc = endpoint.NewService(endpoint.Deps{Store: e.store, Engine: endpoint.NewCheckEngine(nil, logger), Logger: logger})
	e.svc.SyncAgentEndpoints(ctx, stateAgent, "web", "ext-1", map[string]string{"maintenant.endpoint.http": "http://10.0.0.5:80/health"})
	return e
}

func (e *agentEndpointEnv) probe(status agentpb.EndpointStatus) {
	e.t.Helper()
	require.NoError(e.t, e.svc.HandleAgentEvent(e.ctx, stateAgent, &agentpb.EndpointEvent{
		Url: "http://10.0.0.5:80/health", Status: status, StatusCode: 200,
	}, agentevent.Meta{ObservedAt: time.Now()}))
}

func (e *agentEndpointEnv) container(state agentpb.ContainerState) *agentpb.ContainerEvent {
	return &agentpb.ContainerEvent{ContainerId: "ext-1", Name: "web", State: state}
}

func (e *agentEndpointEnv) endpoints() []*endpoint.Endpoint {
	e.t.Helper()
	agentID := stateAgent
	eps, err := e.store.ListEndpoints(e.ctx, endpoint.ListEndpointsOpts{AgentFilter: &agentID})
	require.NoError(e.t, err)
	return eps
}

func (e *agentEndpointEnv) only() *endpoint.Endpoint {
	e.t.Helper()
	eps := e.endpoints()
	require.Len(e.t, eps, 1)
	return eps[0]
}

func TestAgentContainerStop_SetsItsEndpointsUnknownUntilItRunsAgain(t *testing.T) {
	e := newAgentEndpointEnv(t)
	e.probe(agentpb.EndpointStatus_ENDPOINT_STATUS_UP)
	require.Equal(t, endpoint.StatusUp, e.only().Status)

	e.svc.HandleAgentContainerState(e.ctx, stateAgent, e.container(agentpb.ContainerState_CONTAINER_STATE_EXITED))
	ep := e.only()
	assert.Equal(t, endpoint.StatusUnknown, ep.Status, "the endpoint of a stopped container is no longer up")
	assert.Equal(t, "container stopped", ep.LastError)

	e.probe(agentpb.EndpointStatus_ENDPOINT_STATUS_UP)
	assert.Equal(t, endpoint.StatusUnknown, e.only().Status, "a probe result for a stopped container does not bring the endpoint back")

	e.svc.HandleAgentContainerState(e.ctx, stateAgent, e.container(agentpb.ContainerState_CONTAINER_STATE_RUNNING))
	e.probe(agentpb.EndpointStatus_ENDPOINT_STATUS_UP)
	assert.Equal(t, endpoint.StatusUp, e.only().Status, "the first probe after the container runs again is applied")
}

func TestRetireAgentContainers_DeactivatesTheEndpointsOfAVanishedContainer(t *testing.T) {
	e := newAgentEndpointEnv(t)
	e.svc.SyncAgentEndpoints(e.ctx, stateAgent, "api", "ext-2", map[string]string{"maintenant.endpoint.http": "http://10.0.0.6:80/health"})
	e.probe(agentpb.EndpointStatus_ENDPOINT_STATUS_UP)
	require.Len(t, e.endpoints(), 2)

	e.svc.RetireAgentContainers(e.ctx, stateAgent, map[string]struct{}{"ext-2": {}})

	left := e.only()
	assert.Equal(t, "ext-2", left.ExternalID, "only the endpoints of containers the inventory still lists stay")
}
