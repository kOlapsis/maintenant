// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type swarmFakeAPI struct {
	fakeAPI
	services   map[string]map[string]string
	inspectErr error
	inspects   int
}

func (f *swarmFakeAPI) ServiceInspect(_ context.Context, id string, _ client.ServiceInspectOptions) (client.ServiceInspectResult, error) {
	f.inspects++
	if f.inspectErr != nil {
		return client.ServiceInspectResult{}, f.inspectErr
	}
	return client.ServiceInspectResult{Service: swarm.Service{
		ID:   id,
		Spec: swarm.ServiceSpec{Annotations: swarm.Annotations{Labels: f.services[id]}},
	}}, nil
}

func taskSummary() container.Summary {
	return container.Summary{
		ID:    "0123456789abcdef",
		Names: []string{"/prod_web.1.x1y2z3"},
		State: "running",
		Labels: map[string]string{
			"com.docker.swarm.service.id":   "svc1",
			"com.docker.swarm.service.name": "prod_web",
			"com.docker.stack.namespace":    "prod",
			"maintenant.alert.severity":     "info",
		},
	}
}

func TestDiscoverAllWithLabels_ReadsSwarmServiceLabels(t *testing.T) {
	api := &swarmFakeAPI{
		fakeAPI: fakeAPI{list: []container.Summary{taskSummary()}},
		services: map[string]map[string]string{"svc1": {
			"maintenant.alert.severity":                   "critical",
			"maintenant.group":                            "shop",
			"traefik.http.routers.web.rule":               "Host(`shop.example.com`)",
			"traefik.http.routers.web.tls.certresolver":   "le",
			"maintenant.update.tag-include":               `^\d+\.\d+$`,
			"com.docker.stack.namespace":                  "prod",
			"com.docker.swarm.service.name":               "service-value",
			"maintenant.endpoint.http.expected-status":    "200",
			"maintenant.tls.certificates":                 "api.example.com",
			"maintenant.alert.restart_threshold":          "5",
			"traefik.http.routers.web.entrypoints":        "websecure",
			"traefik.http.services.web.loadbalancer.port": "8080",
		}},
	}
	c := &Client{cli: api, logger: newFakeClient(&api.fakeAPI).logger}
	c.SetProxyLabels(true)

	results, err := c.DiscoverAllWithLabels(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)
	got := results[0]

	assert.Equal(t, "prod", got.Container.OrchestrationGroup, "grouped by stack without a Compose project")
	assert.Equal(t, "shop", got.Container.CustomGroup)
	assert.Equal(t, 5, got.Container.RestartThreshold)
	assert.Equal(t, "info", string(got.Container.AlertSeverity), "the container's own label wins")
	assert.Equal(t, "prod_web", got.Labels["com.docker.swarm.service.name"])
	assert.Equal(t, `^\d+\.\d+$`, got.Labels["maintenant.update.tag-include"])
	assert.Equal(t, "https://shop.example.com", got.Labels["maintenant.endpoint.0.http"],
		"Traefik labels set in deploy.labels feed the proxy expansion")
	assert.Contains(t, got.Labels["maintenant.tls.certificates"], "api.example.com")

	_, err = c.DiscoverAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, api.inspects, "service labels are cached between passes")
}

func TestDiscoverAll_IgnoreInDeployLabels(t *testing.T) {
	api := &swarmFakeAPI{
		fakeAPI:  fakeAPI{list: []container.Summary{taskSummary()}},
		services: map[string]map[string]string{"svc1": {"maintenant.ignore": "true"}},
	}
	c := &Client{cli: api, logger: newFakeClient(&api.fakeAPI).logger}

	containers, err := c.DiscoverAll(context.Background())
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.True(t, containers[0].IsIgnored)
}

func TestDiscoverAll_WorkerWithoutServiceAccess(t *testing.T) {
	api := &swarmFakeAPI{
		fakeAPI:    fakeAPI{list: []container.Summary{taskSummary()}},
		inspectErr: errors.New("This node is not a swarm manager"),
	}
	c := &Client{cli: api, logger: newFakeClient(&api.fakeAPI).logger}

	containers, err := c.DiscoverAll(context.Background())
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.False(t, containers[0].IsIgnored)
	assert.Equal(t, "prod", containers[0].OrchestrationGroup)
	assert.Equal(t, "info", string(containers[0].AlertSeverity))
}

func TestStreamEvents_ReadsSwarmServiceLabels(t *testing.T) {
	api := &swarmFakeAPI{
		fakeAPI:  fakeAPI{events: make(chan events.Message, 1)},
		services: map[string]map[string]string{"svc1": {"maintenant.ignore": "true"}},
	}
	api.events <- events.Message{
		Type:   events.ContainerEventType,
		Action: "start",
		Actor: events.Actor{
			ID:         "ctr1",
			Attributes: map[string]string{"name": "prod_web.1.x1y2z3", "com.docker.swarm.service.id": "svc1"},
		},
		Time: 1,
	}
	c := &Client{cli: api, logger: newFakeClient(&api.fakeAPI).logger}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	select {
	case evt := <-c.StreamEvents(ctx):
		assert.Equal(t, "true", evt.Labels["maintenant.ignore"])
		assert.Equal(t, "prod_web.1.x1y2z3", evt.Labels["name"])
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
}
