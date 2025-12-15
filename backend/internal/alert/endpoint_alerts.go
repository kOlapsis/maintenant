package alert

import (
	"time"

	"github.com/kolapsis/pulseboard/internal/endpoint"
)

// EndpointAlert represents an alert or recovery event for an endpoint.
type EndpointAlert struct {
	EndpointID    int64
	ContainerName string
	Target        string
	Type          string // "alert" or "recovery"
	NewAlertState endpoint.AlertState
	Failures      int
	Successes     int
	Threshold     int
	LastError     string
	Timestamp     time.Time
}

// EndpointAlertDetector evaluates check results against thresholds.
type EndpointAlertDetector struct{}

// NewEndpointAlertDetector creates a new detector.
func NewEndpointAlertDetector() *EndpointAlertDetector {
	return &EndpointAlertDetector{}
}

// EvaluateCheckResult checks if a threshold has been crossed and returns an alert if so.
// Returns nil if no alert/recovery needs to be emitted.
func (d *EndpointAlertDetector) EvaluateCheckResult(ep *endpoint.Endpoint, result endpoint.CheckResult) *EndpointAlert {
	now := time.Now()

	if !result.Success {
		// Failure path
		if ep.ConsecutiveFailures >= ep.Config.FailureThreshold && ep.AlertState == endpoint.AlertNormal {
			return &EndpointAlert{
				EndpointID:    ep.ID,
				ContainerName: ep.ContainerName,
				Target:        ep.Target,
				Type:          "alert",
				NewAlertState: endpoint.AlertAlerting,
				Failures:      ep.ConsecutiveFailures,
				Threshold:     ep.Config.FailureThreshold,
				LastError:     result.ErrorMessage,
				Timestamp:     now,
			}
		}
	} else {
		// Success path
		if ep.ConsecutiveSuccesses >= ep.Config.RecoveryThreshold && ep.AlertState == endpoint.AlertAlerting {
			return &EndpointAlert{
				EndpointID:    ep.ID,
				ContainerName: ep.ContainerName,
				Target:        ep.Target,
				Type:          "recovery",
				NewAlertState: endpoint.AlertNormal,
				Successes:     ep.ConsecutiveSuccesses,
				Threshold:     ep.Config.RecoveryThreshold,
				Timestamp:     now,
			}
		}
	}

	return nil
}
