// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/runtime"
)

func TestDockerInsights_IgnoredContainerHasNone(t *testing.T) {
	cfg := &docker.SecurityConfig{
		Privileged:   true,
		PortBindings: []docker.PortBindingInfo{{HostIP: "0.0.0.0", HostPort: "5432", ContainerPort: 5432, Protocol: "tcp"}},
	}

	monitored := &container.Container{ID: "c1", Name: "db"}
	assert.NotEmpty(t, dockerInsights(monitored, cfg, time.Now()))

	ignored := &container.Container{ID: "c1", Name: "db", IsIgnored: true}
	assert.Empty(t, dockerInsights(ignored, cfg, time.Now()))
}

func TestSwarmTaskFailure(t *testing.T) {
	task := map[string]string{
		"com.docker.swarm.service.id":   "svc1",
		"com.docker.swarm.service.name": "prod_web",
	}
	id, name, ok := swarmTaskFailure(runtime.RuntimeEvent{Action: "die", Labels: task})
	assert.True(t, ok)
	assert.Equal(t, "svc1", id)
	assert.Equal(t, "prod_web", name)

	_, _, ok = swarmTaskFailure(runtime.RuntimeEvent{Action: "start", Labels: task})
	assert.False(t, ok)

	_, _, ok = swarmTaskFailure(runtime.RuntimeEvent{Action: "die", Labels: map[string]string{"name": "standalone"}})
	assert.False(t, ok)

	ignored := map[string]string{"com.docker.swarm.service.id": "svc1", "maintenant.ignore": "true"}
	_, _, ok = swarmTaskFailure(runtime.RuntimeEvent{Action: "die", Labels: ignored})
	assert.False(t, ok, "an ignored task feeds no crash-loop alert")
}
