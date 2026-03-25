// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package swarm

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

const (
	crashLoopThreshold    = 3                // failures needed to trigger
	crashLoopWindow       = 5 * time.Minute  // sliding window for counting
	crashLoopRecoveryTime = 10 * time.Minute // quiet period to recover
	crashLoopBufferMax    = 30 * time.Minute // max buffer retention
)

// serviceFailureState tracks failure timestamps for a single service.
type serviceFailureState struct {
	failures    []time.Time
	inCrashLoop bool
	lastFailure time.Time
}

// CrashLoopDetector detects crash-loop patterns per service.
type CrashLoopDetector struct {
	mu       sync.Mutex
	services map[string]*serviceFailureState // keyed by service ID
	logger   *slog.Logger
	callback EventCallback
	alertCb  NodeAlertCallback
}

// NewCrashLoopDetector creates a new crash-loop detector.
func NewCrashLoopDetector(logger *slog.Logger) *CrashLoopDetector {
	return &CrashLoopDetector{
		services: make(map[string]*serviceFailureState),
		logger:   logger,
	}
}

// SetEventCallback sets the SSE event callback.
func (cld *CrashLoopDetector) SetEventCallback(cb EventCallback) {
	cld.callback = cb
}

// SetAlertCallback sets the alert callback.
func (cld *CrashLoopDetector) SetAlertCallback(cb NodeAlertCallback) {
	cld.alertCb = cb
}

// RecordFailure records a task failure for a service and checks for crash-loop.
func (cld *CrashLoopDetector) RecordFailure(serviceID, serviceName, lastError string) {
	cld.mu.Lock()
	defer cld.mu.Unlock()

	now := time.Now()
	state, ok := cld.services[serviceID]
	if !ok {
		state = &serviceFailureState{}
		cld.services[serviceID] = state
	}

	state.failures = append(state.failures, now)
	state.lastFailure = now

	// Prune old failures beyond buffer max.
	cutoff := now.Add(-crashLoopBufferMax)
	pruned := state.failures[:0]
	for _, t := range state.failures {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	state.failures = pruned

	// Count failures within the detection window.
	windowStart := now.Add(-crashLoopWindow)
	count := 0
	for _, t := range state.failures {
		if t.After(windowStart) {
			count++
		}
	}

	if count >= crashLoopThreshold && !state.inCrashLoop {
		state.inCrashLoop = true
		cld.logger.Warn("crash-loop detected",
			"service_id", serviceID, "service_name", serviceName, "failures", count)

		cld.emit(event.SwarmCrashLoopDetected, map[string]interface{}{
			"service_id":     serviceID,
			"service_name":   serviceName,
			"failure_count":  count,
			"window_minutes": int(crashLoopWindow.Minutes()),
			"last_error":     lastError,
			"timestamp":      now.Format(time.RFC3339),
		})

		cld.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "crash_loop",
			Severity:   alert.SeverityCritical,
			Message:    fmt.Sprintf("Swarm service %s is crash-looping (%d failures in %d min)", serviceName, count, int(crashLoopWindow.Minutes())),
			EntityType: "swarm_service",
			EntityName: serviceName,
			Details: map[string]any{
				"service_id":    serviceID,
				"failure_count": count,
				"last_error":    lastError,
			},
			Timestamp: now,
		})
	}
}

// CheckRecoveries checks if any services have recovered from crash-loop.
func (cld *CrashLoopDetector) CheckRecoveries() {
	cld.mu.Lock()
	defer cld.mu.Unlock()

	now := time.Now()
	for serviceID, state := range cld.services {
		if !state.inCrashLoop {
			continue
		}
		if now.Sub(state.lastFailure) >= crashLoopRecoveryTime {
			state.inCrashLoop = false
			cld.logger.Info("crash-loop recovered", "service_id", serviceID)

			cld.emit(event.SwarmCrashLoopRecovered, map[string]interface{}{
				"service_id":   serviceID,
				"service_name": serviceID, // best effort — name may not be available
				"timestamp":    now.Format(time.RFC3339),
			})

			cld.sendAlert(alert.Event{
				Source:     "swarm",
				AlertType:  "crash_loop",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Swarm service crash-loop resolved for %s", serviceID),
				EntityType: "swarm_service",
				EntityName: serviceID,
				Timestamp:  now,
			})
		}
	}
}

// IsCrashLooping returns whether a service is currently in crash-loop state.
func (cld *CrashLoopDetector) IsCrashLooping(serviceID string) bool {
	cld.mu.Lock()
	defer cld.mu.Unlock()
	if state, ok := cld.services[serviceID]; ok {
		return state.inCrashLoop
	}
	return false
}

func (cld *CrashLoopDetector) emit(eventType string, data interface{}) {
	if cld.callback != nil {
		cld.callback(eventType, data)
	}
}

func (cld *CrashLoopDetector) sendAlert(evt alert.Event) {
	if cld.alertCb != nil {
		cld.alertCb(evt)
	}
}
