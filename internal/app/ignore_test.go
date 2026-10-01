// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
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
