// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
)

const defaultReplicaAlertDelay = 5 * time.Minute

// replicaState tracks when a service first became under-replicated.
type replicaState struct {
	firstSeen time.Time
	alerted   bool
}

// ReplicaHealthChecker detects sustained under-replication and emits alerts
// after a configurable delay (default 5 minutes).
type ReplicaHealthChecker struct {
	mu         sync.Mutex
	services   map[string]*replicaState // keyed by service ID
	alertDelay time.Duration
	logger     *slog.Logger
	callback   EventCallback
	alertCb    NodeAlertCallback
}

// NewReplicaHealthChecker creates a new replica health checker.
func NewReplicaHealthChecker(logger *slog.Logger) *ReplicaHealthChecker {
	return &ReplicaHealthChecker{
		services:   make(map[string]*replicaState),
		alertDelay: defaultReplicaAlertDelay,
		logger:     logger,
	}
}

// SetAlertDelay overrides the default alert delay for testing.
func (rhc *ReplicaHealthChecker) SetAlertDelay(d time.Duration) {
	rhc.alertDelay = d
}

// SetEventCallback sets the SSE event callback.
func (rhc *ReplicaHealthChecker) SetEventCallback(cb EventCallback) {
	rhc.callback = cb
}

// SetAlertCallback sets the alert callback.
func (rhc *ReplicaHealthChecker) SetAlertCallback(cb NodeAlertCallback) {
	rhc.alertCb = cb
}

// Check evaluates replica health for all services and emits alerts
// for services that have been under-replicated beyond the alert delay.
func (rhc *ReplicaHealthChecker) Check(services []*SwarmService) {
	rhc.mu.Lock()
	defer rhc.mu.Unlock()

	now := time.Now()
	activeServiceIDs := make(map[string]bool, len(services))

	for _, svc := range services {
		activeServiceIDs[svc.ServiceID] = true

		if svc.Mode != "replicated" || svc.DesiredReplicas == 0 || container.IgnoredByLabels(svc.Labels) {
			rhc.recover(svc.ServiceID, svc.Name, now)
			continue
		}

		underReplicated := svc.RunningReplicas < svc.DesiredReplicas
		state, tracked := rhc.services[svc.ServiceID]

		if underReplicated {
			if !tracked {
				// First time seeing this service under-replicated.
				rhc.services[svc.ServiceID] = &replicaState{
					firstSeen: now,
					alerted:   false,
				}
				continue
			}
			if !state.alerted && now.Sub(state.firstSeen) >= rhc.alertDelay {
				state.alerted = true
				rhc.logger.Warn("sustained under-replication detected",
					"service", svc.Name,
					"running", svc.RunningReplicas,
					"desired", svc.DesiredReplicas,
					"duration", now.Sub(state.firstSeen).Round(time.Second),
				)
				rhc.emitAlert(svc, now)
			}
		} else {
			// Service is healthy — recover if previously tracked.
			rhc.recover(svc.ServiceID, svc.Name, now)
		}
	}

	// Clean up services that no longer exist.
	for serviceID := range rhc.services {
		if !activeServiceIDs[serviceID] {
			delete(rhc.services, serviceID)
		}
	}
}

func (rhc *ReplicaHealthChecker) recover(serviceID, serviceName string, now time.Time) {
	state, tracked := rhc.services[serviceID]
	if !tracked {
		return
	}
	wasAlerted := state.alerted
	delete(rhc.services, serviceID)

	if wasAlerted {
		rhc.logger.Info("under-replication resolved", "service", serviceName)
		rhc.emitRecovery(serviceID, serviceName, now)
	}
}

func (rhc *ReplicaHealthChecker) emitAlert(svc *SwarmService, now time.Time) {
	if rhc.callback != nil {
		rhc.callback(event.SwarmServiceUpdated, map[string]interface{}{
			"service_id":       svc.ServiceID,
			"name":             svc.Name,
			"running_replicas": svc.RunningReplicas,
			"desired_replicas": svc.DesiredReplicas,
			"replica_alert":    true,
		})
	}
	if rhc.alertCb != nil {
		rhc.alertCb(alert.Event{
			Source:     "swarm",
			AlertType:  "replica_unhealthy",
			Severity:   alert.SeverityWarning,
			Message:    fmt.Sprintf("Swarm service %s under-replicated for %s: %d/%d replicas", svc.Name, rhc.alertDelay, svc.RunningReplicas, svc.DesiredReplicas),
			EntityType: "swarm_service",
			EntityID:   svc.ServiceID,
			EntityName: svc.Name,
			Details: map[string]any{
				"service_id":       svc.ServiceID,
				"running_replicas": svc.RunningReplicas,
				"desired_replicas": svc.DesiredReplicas,
			},
			Timestamp: now,
		})
	}
}

func (rhc *ReplicaHealthChecker) emitRecovery(serviceID, serviceName string, now time.Time) {
	if rhc.alertCb != nil {
		rhc.alertCb(alert.Event{
			Source:     "swarm",
			AlertType:  "replica_unhealthy",
			Severity:   alert.SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Swarm service %s replicas restored", serviceName),
			EntityType: "swarm_service",
			EntityID:   serviceID,
			EntityName: serviceName,
			Details: map[string]any{
				"service_id": serviceID,
			},
			Timestamp: now,
		})
	}
}
