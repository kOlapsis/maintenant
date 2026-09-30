// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/status"
)

// countEvents drains what the broker already delivered; broadcasting is synchronous.
func countEvents(ch chan v1.SSEEvent) map[string]int {
	seen := map[string]int{}
	for {
		select {
		case evt := <-ch:
			seen[evt.Type]++
		default:
			return seen
		}
	}
}

func TestEndpointEvents_ReachTheBrokersAndTheStatusPage(t *testing.T) {
	a, _ := newTestApp(t, nil)
	ctx := context.Background()
	admin := make(chan v1.SSEEvent, 256)
	a.broker.AddObserver(admin)
	public := make(chan v1.SSEEvent, 256)
	a.statusBroker.AddObserver(public)

	ep, err := a.endpointSvc.CreateStandalone(ctx, "site", "https://example.com", endpoint.TypeHTTP, endpoint.DefaultConfig())
	require.NoError(t, err)
	_, err = a.statusCompStore.CreateComponent(ctx, &status.Component{
		CompositionMode: status.CompositionExplicit, DisplayName: "Site", Visible: true,
		Monitors: []status.MonitorRef{{Type: "endpoint", ID: ep.ID}},
	})
	require.NoError(t, err)

	for range ep.Config.FailureThreshold {
		a.endpointSvc.ProcessCheckResult(ctx, ep.ID, endpoint.CheckResult{
			EndpointID: ep.ID, ErrorMessage: "connection refused", Timestamp: time.Now(),
		})
	}

	seen := countEvents(admin)
	assert.Equal(t, 1, seen["endpoint.discovered"])
	assert.Equal(t, 1, seen["endpoint.status_changed"], "unknown to down, then down stays down")
	assert.Equal(t, 1, seen["endpoint.alert"])
	assert.Equal(t, 1, seen["status.component_changed"], "the admin bus carries the status page events")
	assert.Equal(t, 1, countEvents(public)["status.component_changed"], "a component changes once, with its monitor")
}
