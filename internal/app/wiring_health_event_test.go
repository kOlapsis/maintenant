// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestContainerHealthEvent(t *testing.T) {
	h := func(s container.HealthStatus) *container.HealthStatus { return &s }

	cases := []struct {
		name        string
		prev        *container.HealthStatus
		next        container.HealthStatus
		wantRaised  bool
		wantRecover bool
	}{
		{"healthy to unhealthy alerts", h(container.HealthHealthy), container.HealthUnhealthy, true, false},
		{"starting to unhealthy alerts", h(container.HealthStarting), container.HealthUnhealthy, true, false},
		{"no previous health to unhealthy alerts", nil, container.HealthUnhealthy, true, false},
		{"unhealthy to unhealthy stays quiet", h(container.HealthUnhealthy), container.HealthUnhealthy, false, false},
		{"unhealthy to healthy recovers", h(container.HealthUnhealthy), container.HealthHealthy, true, true},
		{"starting to healthy recovers", h(container.HealthStarting), container.HealthHealthy, true, true},
		{"no previous health to healthy recovers", nil, container.HealthHealthy, true, true},
		{"unhealthy to starting stays quiet", h(container.HealthUnhealthy), container.HealthStarting, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evt, raised := containerHealthEvent(map[string]any{
				"id":              "c1",
				"container_name":  "web",
				"previous_health": tc.prev,
				"health_status":   tc.next,
			})
			assert.Equal(t, tc.wantRaised, raised)
			if raised {
				assert.Equal(t, tc.wantRecover, evt.IsRecover)
				assert.Equal(t, "health_unhealthy", evt.AlertType)
			}
		})
	}
}
