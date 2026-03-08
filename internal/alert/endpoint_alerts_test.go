// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package alert

import (
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// baseEndpoint returns an Endpoint with sane defaults for alert threshold tests.
// FailureThreshold=3, RecoveryThreshold=2 match DefaultConfig.
func baseEndpoint() *endpoint.Endpoint {
	cfg := endpoint.DefaultConfig()
	return &endpoint.Endpoint{
		ID:            1,
		ContainerName: "test-container",
		Target:        "https://example.com/health",
		Config:        cfg,
		AlertState:    endpoint.AlertNormal,
		Status:        endpoint.StatusUp,
	}
}

func failureResult(msg string) endpoint.CheckResult {
	return endpoint.CheckResult{
		EndpointID:   1,
		Success:      false,
		ErrorMessage: msg,
		Timestamp:    time.Now(),
	}
}

func successResult() endpoint.CheckResult {
	return endpoint.CheckResult{
		EndpointID: 1,
		Success:    true,
		Timestamp:  time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Failure path — threshold logic
// ---------------------------------------------------------------------------

func TestEndpointAlertDetector_FailuresBelowThreshold_NoAlert(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	// Threshold is 3; set consecutive failures below it
	ep.ConsecutiveFailures = ep.Config.FailureThreshold - 1 // 2

	result := d.EvaluateCheckResult(ep, failureResult("timeout"))

	assert.Nil(t, result,
		"no alert should be emitted when consecutive failures (%d) < threshold (%d)",
		ep.ConsecutiveFailures, ep.Config.FailureThreshold)
}

func TestEndpointAlertDetector_FailuresAtThreshold_FiresAlert(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	ep.AlertState = endpoint.AlertNormal
	ep.ConsecutiveFailures = ep.Config.FailureThreshold // exactly at threshold

	result := d.EvaluateCheckResult(ep, failureResult("connection refused"))

	require.NotNil(t, result, "alert should fire when consecutive failures reach threshold")
	assert.Equal(t, "alert", result.Type)
	assert.Equal(t, endpoint.AlertAlerting, result.NewAlertState)
	assert.Equal(t, ep.ID, result.EndpointID)
	assert.Equal(t, ep.ContainerName, result.ContainerName)
	assert.Equal(t, ep.Target, result.Target)
	assert.Equal(t, ep.ConsecutiveFailures, result.Failures)
	assert.Equal(t, ep.Config.FailureThreshold, result.Threshold)
	assert.Equal(t, "connection refused", result.LastError)
	assert.WithinDuration(t, time.Now(), result.Timestamp, 5*time.Second)
}

func TestEndpointAlertDetector_FailuresAboveThreshold_FiresAlert(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	ep.AlertState = endpoint.AlertNormal
	ep.ConsecutiveFailures = ep.Config.FailureThreshold + 5 // well above threshold

	result := d.EvaluateCheckResult(ep, failureResult("DNS NXDOMAIN"))

	require.NotNil(t, result, "alert should fire when consecutive failures exceed threshold")
	assert.Equal(t, "alert", result.Type)
}

func TestEndpointAlertDetector_FailuresAlreadyAlerting_NoAlert(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	// Already alerting — further failures must be deduped
	ep.AlertState = endpoint.AlertAlerting
	ep.ConsecutiveFailures = ep.Config.FailureThreshold + 10

	result := d.EvaluateCheckResult(ep, failureResult("still down"))

	assert.Nil(t, result,
		"no alert should fire when endpoint is already in AlertAlerting state (dedup)")
}

// ---------------------------------------------------------------------------
// Recovery path — threshold logic
// ---------------------------------------------------------------------------

func TestEndpointAlertDetector_RecoveryAtThreshold_FiresRecovery(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	ep.AlertState = endpoint.AlertAlerting
	ep.ConsecutiveSuccesses = ep.Config.RecoveryThreshold // exactly at recovery threshold

	result := d.EvaluateCheckResult(ep, successResult())

	require.NotNil(t, result, "recovery should fire when consecutive successes reach recovery threshold")
	assert.Equal(t, "recovery", result.Type)
	assert.Equal(t, endpoint.AlertNormal, result.NewAlertState)
	assert.Equal(t, ep.ID, result.EndpointID)
	assert.Equal(t, ep.ContainerName, result.ContainerName)
	assert.Equal(t, ep.Target, result.Target)
	assert.Equal(t, ep.ConsecutiveSuccesses, result.Successes)
	assert.Equal(t, ep.Config.RecoveryThreshold, result.Threshold)
	assert.WithinDuration(t, time.Now(), result.Timestamp, 5*time.Second)
}

func TestEndpointAlertDetector_RecoveryNotAlerting_NoRecovery(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	// Endpoint is healthy — was never alerting
	ep.AlertState = endpoint.AlertNormal
	ep.ConsecutiveSuccesses = ep.Config.RecoveryThreshold + 5

	result := d.EvaluateCheckResult(ep, successResult())

	assert.Nil(t, result,
		"no recovery event should fire when endpoint alert state is already AlertNormal")
}

func TestEndpointAlertDetector_SuccessBelowRecoveryThreshold_NoRecovery(t *testing.T) {
	d := NewEndpointAlertDetector()
	ep := baseEndpoint()
	ep.AlertState = endpoint.AlertAlerting
	ep.ConsecutiveSuccesses = ep.Config.RecoveryThreshold - 1 // 1, below threshold of 2

	result := d.EvaluateCheckResult(ep, successResult())

	assert.Nil(t, result,
		"no recovery should fire when consecutive successes (%d) < recovery threshold (%d)",
		ep.ConsecutiveSuccesses, ep.Config.RecoveryThreshold)
}
