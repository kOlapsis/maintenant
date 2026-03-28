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

package kubernetes

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

const (
	// Default thresholds for alert detection.
	defaultReplicaGracePeriod = 5 * time.Minute
	defaultCrashLoopRestarts  = 3
	defaultCrashLoopWindow    = 10 * time.Minute
)

// K8sAlertCallback is the function signature for emitting alert events.
type K8sAlertCallback func(evt alert.Event)

// K8sAlertChecker detects alert conditions in Kubernetes workloads and nodes.
type K8sAlertChecker struct {
	mu      sync.Mutex
	logger  *slog.Logger
	alertCb K8sAlertCallback

	// replicaAlerts tracks when a workload first became under-replicated.
	replicaAlerts map[string]time.Time // workload ID -> first seen under-replicated

	// crashLoopPods tracks recent restart counts per pod.
	crashLoopPods map[string]*podRestartState // pod key (ns/name) -> state

	// nodeConditions tracks when a node condition was first detected.
	nodeConditions map[string]time.Time // "nodeName:conditionType" -> first seen
}

// podRestartState tracks restart count observations for crash-loop detection.
type podRestartState struct {
	lastCount      int32
	observations   []time.Time
	alertedCrashLoop bool // true when a CrashLoopBackOff alert was already emitted
}

// NewK8sAlertChecker creates a new K8sAlertChecker.
func NewK8sAlertChecker(logger *slog.Logger) *K8sAlertChecker {
	return &K8sAlertChecker{
		logger:         logger,
		replicaAlerts:  make(map[string]time.Time),
		crashLoopPods:  make(map[string]*podRestartState),
		nodeConditions: make(map[string]time.Time),
	}
}

// SetAlertCallback sets the callback for emitting alerts.
func (c *K8sAlertChecker) SetAlertCallback(cb K8sAlertCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alertCb = cb
}

// CheckWorkloadReplicas checks for workloads where ready < desired for longer
// than the grace period. Returns IDs of workloads that should alert.
func (c *K8sAlertChecker) CheckWorkloadReplicas(workloads []K8sWorkload) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	activeIDs := make(map[string]bool, len(workloads))

	for _, wl := range workloads {
		activeIDs[wl.ID] = true

		if wl.DesiredReplicas == 0 || wl.ReadyReplicas >= wl.DesiredReplicas {
			// Recovered or scaled to zero — remove tracking.
			if _, ok := c.replicaAlerts[wl.ID]; ok {
				delete(c.replicaAlerts, wl.ID)
				c.emitAlert(alert.Event{
					Source:     "kubernetes",
					AlertType:  "replica_health",
					Severity:   "info",
					IsRecover:  true,
					Message:    fmt.Sprintf("Workload %s recovered: %d/%d replicas ready", wl.Name, wl.ReadyReplicas, wl.DesiredReplicas),
					EntityType: "workload",
					EntityName: wl.ID,
					Details: map[string]any{
						"namespace":        wl.Namespace,
						"kind":             wl.Kind,
						"ready_replicas":   wl.ReadyReplicas,
						"desired_replicas": wl.DesiredReplicas,
					},
					Timestamp: now,
				})
			}
			continue
		}

		// Under-replicated.
		firstSeen, tracked := c.replicaAlerts[wl.ID]
		if !tracked {
			c.replicaAlerts[wl.ID] = now
			continue
		}

		if now.Sub(firstSeen) >= defaultReplicaGracePeriod {
			severity := "warning"
			if wl.ReadyReplicas == 0 {
				severity = "critical"
			}
			c.emitAlert(alert.Event{
				Source:     "kubernetes",
				AlertType:  "replica_health",
				Severity:   severity,
				Message:    fmt.Sprintf("Workload %s under-replicated for %s: %d/%d ready", wl.Name, now.Sub(firstSeen).Truncate(time.Second), wl.ReadyReplicas, wl.DesiredReplicas),
				EntityType: "workload",
				EntityName: wl.ID,
				Details: map[string]any{
					"namespace":        wl.Namespace,
					"kind":             wl.Kind,
					"ready_replicas":   wl.ReadyReplicas,
					"desired_replicas": wl.DesiredReplicas,
					"duration":         now.Sub(firstSeen).String(),
				},
				Timestamp: now,
			})
		}
	}

	// Clean up workloads that no longer exist.
	for id := range c.replicaAlerts {
		if !activeIDs[id] {
			delete(c.replicaAlerts, id)
		}
	}
}

// CheckCrashLoopBackOff checks pods for crash-loop patterns based on restart
// counts. A pod is considered in crash-loop when it accumulates
// defaultCrashLoopRestarts or more restart increments within defaultCrashLoopWindow.
func (c *K8sAlertChecker) CheckCrashLoopBackOff(pods []K8sPod) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-defaultCrashLoopWindow)
	activePods := make(map[string]bool, len(pods))

	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		activePods[key] = true

		isCrashLoop := pod.StatusReason == "CrashLoopBackOff"

		state, exists := c.crashLoopPods[key]
		if !exists {
			state = &podRestartState{
				lastCount: pod.RestartCount,
			}
			c.crashLoopPods[key] = state

			// Pod was already in CrashLoopBackOff when Maintenant first saw it — alert immediately.
			if isCrashLoop {
				state.alertedCrashLoop = true
				c.emitAlert(alert.Event{
					Source:     "kubernetes",
					AlertType:  "crash_loop",
					Severity:   "critical",
					Message:    fmt.Sprintf("Pod %s/%s is in CrashLoopBackOff (%d restarts)", pod.Namespace, pod.Name, pod.RestartCount),
					EntityType: "pod",
					EntityName: key,
					Details: map[string]any{
						"namespace":     pod.Namespace,
						"restart_count": pod.RestartCount,
						"status_reason": pod.StatusReason,
						"node_name":     pod.NodeName,
					},
					Timestamp: now,
				})
			}
			continue
		}

		// Recovery: pod left CrashLoopBackOff.
		if !isCrashLoop && state.alertedCrashLoop {
			state.alertedCrashLoop = false
			state.observations = nil
			c.emitAlert(alert.Event{
				Source:     "kubernetes",
				AlertType:  "crash_loop",
				Severity:   "info",
				IsRecover:  true,
				Message:    fmt.Sprintf("Pod %s/%s recovered from CrashLoopBackOff", pod.Namespace, pod.Name),
				EntityType: "pod",
				EntityName: key,
				Details: map[string]any{
					"namespace":     pod.Namespace,
					"restart_count": pod.RestartCount,
					"node_name":     pod.NodeName,
				},
				Timestamp: now,
			})
		}

		// Detect restart increment.
		if pod.RestartCount > state.lastCount {
			state.observations = append(state.observations, now)
			state.lastCount = pod.RestartCount
		}

		// Prune old observations outside the window.
		pruned := state.observations[:0]
		for _, t := range state.observations {
			if t.After(cutoff) {
				pruned = append(pruned, t)
			}
		}
		state.observations = pruned

		// Check increment threshold (catches fast crash loops not yet at CrashLoopBackOff status).
		if !state.alertedCrashLoop && len(state.observations) >= defaultCrashLoopRestarts {
			state.alertedCrashLoop = true
			c.emitAlert(alert.Event{
				Source:     "kubernetes",
				AlertType:  "crash_loop",
				Severity:   "critical",
				Message:    fmt.Sprintf("Pod %s/%s in crash loop: %d restarts in %s", pod.Namespace, pod.Name, len(state.observations), defaultCrashLoopWindow),
				EntityType: "pod",
				EntityName: key,
				Details: map[string]any{
					"namespace":     pod.Namespace,
					"restart_count": pod.RestartCount,
					"status_reason": pod.StatusReason,
					"node_name":     pod.NodeName,
				},
				Timestamp: now,
			})
		}
	}

	// Clean up pods that no longer exist.
	for key := range c.crashLoopPods {
		if !activePods[key] {
			delete(c.crashLoopPods, key)
		}
	}
}

// CheckNodeConditions checks nodes for unhealthy conditions: NotReady,
// MemoryPressure, DiskPressure, PIDPressure.
func (c *K8sAlertChecker) CheckNodeConditions(nodes []K8sNode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	activeKeys := make(map[string]bool)

	for _, node := range nodes {
		// NotReady.
		if node.Status == "not-ready" {
			key := node.Name + ":NotReady"
			activeKeys[key] = true
			if _, tracked := c.nodeConditions[key]; !tracked {
				c.nodeConditions[key] = now
				c.emitAlert(alert.Event{
					Source:     "kubernetes",
					AlertType:  "node_condition",
					Severity:   "critical",
					Message:    fmt.Sprintf("Node %s is NotReady", node.Name),
					EntityType: "node",
					EntityName: node.Name,
					Details: map[string]any{
						"condition": "NotReady",
						"roles":     node.Roles,
					},
					Timestamp: now,
				})
			}
		} else {
			// Node recovered from NotReady.
			key := node.Name + ":NotReady"
			if _, tracked := c.nodeConditions[key]; tracked {
				delete(c.nodeConditions, key)
				c.emitAlert(alert.Event{
					Source:     "kubernetes",
					AlertType:  "node_condition",
					Severity:   "info",
					IsRecover:  true,
					Message:    fmt.Sprintf("Node %s recovered: now Ready", node.Name),
					EntityType: "node",
					EntityName: node.Name,
					Details: map[string]any{
						"condition": "Ready",
						"roles":     node.Roles,
					},
					Timestamp: now,
				})
			}
		}

		// Pressure conditions.
		for _, cond := range node.Conditions {
			if cond.Type != "MemoryPressure" && cond.Type != "DiskPressure" && cond.Type != "PIDPressure" {
				continue
			}
			key := node.Name + ":" + cond.Type
			if cond.Status == "True" {
				activeKeys[key] = true
				if _, tracked := c.nodeConditions[key]; !tracked {
					c.nodeConditions[key] = now
					c.emitAlert(alert.Event{
						Source:     "kubernetes",
						AlertType:  "node_condition",
						Severity:   "warning",
						Message:    fmt.Sprintf("Node %s has %s", node.Name, cond.Type),
						EntityType: "node",
						EntityName: node.Name,
						Details: map[string]any{
							"condition": cond.Type,
							"reason":    cond.Reason,
							"message":   cond.Message,
							"roles":     node.Roles,
						},
						Timestamp: now,
					})
				}
			} else {
				// Pressure resolved.
				if _, tracked := c.nodeConditions[key]; tracked {
					delete(c.nodeConditions, key)
					c.emitAlert(alert.Event{
						Source:     "kubernetes",
						AlertType:  "node_condition",
						Severity:   "info",
						IsRecover:  true,
						Message:    fmt.Sprintf("Node %s: %s resolved", node.Name, cond.Type),
						EntityType: "node",
						EntityName: node.Name,
						Details: map[string]any{
							"condition": cond.Type,
							"roles":     node.Roles,
						},
						Timestamp: now,
					})
				}
			}
		}
	}
}

func (c *K8sAlertChecker) emitAlert(evt alert.Event) {
	if c.alertCb != nil {
		c.alertCb(evt)
	} else {
		c.logger.Info("k8s alert (no callback)", "type", evt.AlertType, "severity", evt.Severity, "message", evt.Message)
	}
}
