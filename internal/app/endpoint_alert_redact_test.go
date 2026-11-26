// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/endpoint"
)

func TestEndpointAlerts_ShowTheTargetAsSchemeHostAndPort(t *testing.T) {
	const target = "https://ops:hunter2@example.com:8443/hooks/S3CR3T?token=t0k3n"
	ep := &endpoint.Endpoint{ID: "ep-1", Name: "api", Target: target}
	lastError := `request failed: Get "https://ops:***@example.com:8443/hooks/S3CR3T?token=t0k3n": EOF`
	now := time.Now()

	var built []any
	for _, kind := range []string{"alert", "recovery"} {
		evt, _, data := endpointThresholdAlert(ep, &alert.EndpointAlert{
			EndpointID: "ep-1", Target: target, Type: kind, Failures: 3, Successes: 2, Threshold: 3, LastError: lastError, Timestamp: now,
		})
		assert.Contains(t, evt.Message, "Endpoint https://example.com:8443 ")
		built = append(built, evt, data)
	}
	for _, result := range []endpoint.CheckResult{
		{Degraded: true, DegradedReason: "certificate signed by unknown authority", Timestamp: now},
		{Success: true, Timestamp: now},
	} {
		evt, ok := endpointTrustEvent(ep, result)
		require.True(t, ok)
		assert.Contains(t, evt.Message, "Endpoint https://example.com:8443 ")
		built = append(built, evt)
	}

	text, err := json.Marshal(built)
	require.NoError(t, err)
	for _, secret := range []string{"hunter2", "ops:", "S3CR3T", "t0k3n"} {
		assert.NotContains(t, string(text), secret)
	}
}
