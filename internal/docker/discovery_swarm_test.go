// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

func TestMapFromList_SwarmTaskCarriesItsService(t *testing.T) {
	labels := map[string]string{
		"com.docker.swarm.service.id":   "svc1",
		"com.docker.swarm.service.name": "prod_web",
		"com.docker.swarm.node.id":      "node1",
		"com.docker.swarm.task.id":      "task9",
		"com.docker.swarm.task.name":    "prod_web.2.task9",
		"com.docker.stack.namespace":    "prod",
	}
	c := mapFromList(container.Summary{ID: "abc", Names: []string{"/prod_web.2.task9"}, State: "running"}, labels, time.Now())

	assert.Equal(t, cmodel.ControllerSwarmService, c.ControllerKind)
	assert.Equal(t, "svc1", c.SwarmServiceID)
	assert.Equal(t, "prod_web", c.SwarmServiceName)
	assert.Equal(t, "node1", c.SwarmNodeID)
	assert.Equal(t, 2, c.SwarmTaskSlot)
	assert.Equal(t, "prod", c.OrchestrationGroup)
}
