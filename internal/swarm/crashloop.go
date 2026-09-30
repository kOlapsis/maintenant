// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	dockerswarm "github.com/moby/moby/api/types/swarm"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
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
	name        string
	failures    []time.Time
	inCrashLoop bool
	lastFailure time.Time
}

// CrashLoopDetector detects crash-loop patterns per service.
type CrashLoopDetector struct {
	mu          sync.Mutex
	services    map[string]*serviceFailureState // keyed by service ID
	failedTasks map[string]bool                 // failed task IDs already counted
	logger      *slog.Logger
	callback    EventCallback
	alertCb     NodeAlertCallback
}

// NewCrashLoopDetector creates a new crash-loop detector.
func NewCrashLoopDetector(logger *slog.Logger) *CrashLoopDetector {
	return &CrashLoopDetector{
		services:    make(map[string]*serviceFailureState),
		failedTasks: make(map[string]bool),
		logger:      logger,
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

// Resume takes over the active crash-loop alerts left by a previous run, so a
// service that stays quiet for the recovery time resolves its alert.
func (cld *CrashLoopDetector) Resume(active []*alert.Alert) {
	cld.mu.Lock()
	defer cld.mu.Unlock()

	now := time.Now()
	for _, a := range active {
		if a.Source == "swarm" && a.AlertType == "crash_loop" && a.EntityType == "swarm_service" {
			cld.services[a.EntityID] = &serviceFailureState{name: a.EntityName, inCrashLoop: true, lastFailure: now}
		}
	}
}

// ObserveTasks counts once each task of the snapshot that failed within the detection window, on any node.
func (cld *CrashLoopDetector) ObserveTasks(snap TopologySnapshot) {
	services := make(map[string]*SwarmService, len(snap.Services))
	for i := range snap.Services {
		services[snap.Services[i].ServiceID] = &snap.Services[i]
	}

	cld.mu.Lock()
	defer cld.mu.Unlock()

	now := time.Now()
	failed := make(map[string]bool)
	for _, t := range snap.Tasks {
		svc, known := services[t.ServiceID]
		if !known || !taskFailed(t) {
			continue
		}
		failed[t.TaskID] = true
		at := t.Timestamp
		if at.IsZero() {
			at = now
		}
		if cld.failedTasks[t.TaskID] || container.IgnoredByLabels(svc.Labels) || now.Sub(at) >= crashLoopWindow {
			continue
		}
		cld.emit(event.SwarmTaskFailed, map[string]interface{}{
			"task_id":      t.TaskID,
			"service_id":   t.ServiceID,
			"service_name": svc.Name,
			"node_id":      t.NodeID,
			"container_id": t.ContainerID,
			"error":        t.Error,
			"exit_code":    t.ExitCode,
			"timestamp":    at.Format(time.RFC3339),
		})
		cld.recordFailure(t.ServiceID, svc.Name, t.Error, at, now)
	}
	cld.failedTasks = failed
}

// taskFailed reports a task that stopped while Swarm wanted it running: a rolling update or a scale-down shuts tasks down instead of failing them.
func taskFailed(t SwarmTask) bool {
	if t.State != string(dockerswarm.TaskStateFailed) {
		return false
	}
	// The task API does not report the OOM killer, so a SIGKILL there counts as one.
	return t.ExitCode == nil || !container.IsCleanExit(*t.ExitCode, true)
}

func (cld *CrashLoopDetector) recordFailure(serviceID, serviceName, lastError string, at, now time.Time) {
	state, ok := cld.services[serviceID]
	if !ok {
		state = &serviceFailureState{}
		cld.services[serviceID] = state
	}

	state.name = serviceName
	state.failures = append(state.failures, at)
	if at.After(state.lastFailure) {
		state.lastFailure = at
	}

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
			EntityID:   serviceID,
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
				"service_name": state.name,
				"timestamp":    now.Format(time.RFC3339),
			})

			cld.sendAlert(alert.Event{
				Source:     "swarm",
				AlertType:  "crash_loop",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Swarm service crash-loop resolved for %s", state.name),
				EntityType: "swarm_service",
				EntityID:   serviceID,
				EntityName: state.name,
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
