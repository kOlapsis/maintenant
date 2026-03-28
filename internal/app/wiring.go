// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package app

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/security"
)

// wireAlertCallbacks wires all service event callbacks for SSE broadcasting,
// alert event forwarding, and status page integration.
func (a *App) wireAlertCallbacks(alertDetector *alert.EndpointAlertDetector) {
	ctx := context.Background()
	alertCh := a.alertEngine.EventChannel()

	sendAlert := func(evt alert.Event) {
		alertCh <- evt
		a.statusSvc.HandleAlertEvent(ctx, evt)
	}

	// Container events
	a.containerSvc.SetEventCallback(func(eventType string, data interface{}) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})

		if eventType == "container.state_changed" || eventType == "container.health_changed" {
			if m, ok := data.(map[string]interface{}); ok {
				a.statusSvc.NotifyMonitorChanged(ctx, "container", toInt64(m["id"]))
			}
		}

		switch eventType {
		case "container.restart_alert":
			if ra, ok := data.(*alert.RestartAlert); ok && ra != nil {
				severity := alert.SeverityWarning
				if ra.RestartCount >= ra.Threshold*alert.CriticalRestartMultiplier {
					severity = alert.SeverityCritical
				}
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "restart_loop",
					Severity:   severity,
					Message:    fmt.Sprintf("Container %s exceeded restart threshold (%d/%d)", ra.ContainerName, ra.RestartCount, ra.Threshold),
					EntityType: "container",
					EntityID:   ra.ContainerID,
					EntityName: ra.ContainerName,
					Details: map[string]any{
						"restart_count": ra.RestartCount,
						"threshold":     ra.Threshold,
					},
					Timestamp: ra.Timestamp,
				})
			}
		case "container.restart_recovery":
			if m, ok := data.(map[string]interface{}); ok {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "restart_loop",
					Severity:   alert.SeverityInfo,
					IsRecover:  true,
					Message:    fmt.Sprintf("Container %s restart rate returned to normal", toString(m["container_name"])),
					EntityType: "container",
					EntityID:   toInt64(m["container_id"]),
					EntityName: toString(m["container_name"]),
					Timestamp:  time.Now(),
				})
			}
		case "container.archived":
			if m, ok := data.(map[string]interface{}); ok {
				a.alertEngine.ResolveByEntity(ctx, "container", toInt64(m["id"]))
			}
		case "container.health_changed":
			m, ok := data.(map[string]interface{})
			if !ok {
				return
			}
			prev, _ := m["previous_health"].(*container.HealthStatus)
			newH, _ := m["health_status"].(container.HealthStatus)
			if prev != nil && *prev == container.HealthHealthy && newH == container.HealthUnhealthy {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "health_unhealthy",
					Severity:   alert.SeverityWarning,
					Message:    "Container became unhealthy",
					EntityType: "container",
					EntityID:   toInt64(m["id"]),
					Details:    m,
					Timestamp:  time.Now(),
				})
			} else if prev != nil && *prev == container.HealthUnhealthy && newH == container.HealthHealthy {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "health_unhealthy",
					Severity:   alert.SeverityInfo,
					IsRecover:  true,
					Message:    "Container recovered to healthy",
					EntityType: "container",
					EntityID:   toInt64(m["id"]),
					Details:    m,
					Timestamp:  time.Now(),
				})
			}
		}
	})

	// Endpoint alerts
	a.endpointSvc.SetAlertCallback(func(ep *endpoint.Endpoint, result endpoint.CheckResult) (string, interface{}) {
		a.statusSvc.NotifyMonitorChanged(ctx, "endpoint", ep.ID)

		al := alertDetector.EvaluateCheckResult(ep, result)
		if al == nil {
			return "", nil
		}
		if al.Type == "alert" {
			sendAlert(alert.Event{
				Source:     alert.SourceEndpoint,
				AlertType:  "consecutive_failure",
				Severity:   alert.SeverityCritical,
				Message:    fmt.Sprintf("Endpoint %s failed %d consecutive checks", al.Target, al.Failures),
				EntityType: "endpoint",
				EntityID:   al.EndpointID,
				EntityName: al.ContainerName,
				Details: map[string]any{
					"target":     al.Target,
					"failures":   al.Failures,
					"threshold":  al.Threshold,
					"last_error": al.LastError,
				},
				Timestamp: al.Timestamp,
			})
			return "endpoint.alert", map[string]interface{}{
				"endpoint_id":          al.EndpointID,
				"container_name":       al.ContainerName,
				"target":               al.Target,
				"consecutive_failures": al.Failures,
				"threshold":            al.Threshold,
				"last_error":           al.LastError,
				"timestamp":            al.Timestamp,
			}
		}
		sendAlert(alert.Event{
			Source:     alert.SourceEndpoint,
			AlertType:  "consecutive_failure",
			Severity:   alert.SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Endpoint %s recovered after %d consecutive successes", al.Target, al.Successes),
			EntityType: "endpoint",
			EntityID:   al.EndpointID,
			EntityName: al.ContainerName,
			Details: map[string]any{
				"target":    al.Target,
				"successes": al.Successes,
				"threshold": al.Threshold,
			},
			Timestamp: al.Timestamp,
		})
		return "endpoint.recovery", map[string]interface{}{
			"endpoint_id":           al.EndpointID,
			"container_name":        al.ContainerName,
			"target":                al.Target,
			"consecutive_successes": al.Successes,
			"threshold":             al.Threshold,
			"timestamp":             al.Timestamp,
		}
	})

	// Endpoint removal → certificate monitor cleanup
	a.endpointSvc.SetEndpointRemovedCallback(a.certSvc.DeactivateByEndpointID)

	// Heartbeat alerts
	a.heartbeatSvc.SetAlertCallback(func(h *heartbeat.Heartbeat, alertType string, details map[string]interface{}) {
		a.statusSvc.NotifyMonitorChanged(ctx, "heartbeat", h.ID)

		isRecover := alertType == "recovery"
		severity := alert.SeverityCritical
		msg := fmt.Sprintf("Heartbeat '%s' missed deadline", h.Name)
		if isRecover {
			severity = alert.SeverityInfo
			msg = fmt.Sprintf("Heartbeat '%s' recovered", h.Name)
		}
		hbAlertType := "deadline_missed"
		if t, ok := details["alert_type"].(string); ok {
			hbAlertType = t
		}
		sendAlert(alert.Event{
			Source:     alert.SourceHeartbeat,
			AlertType:  hbAlertType,
			Severity:   severity,
			IsRecover:  isRecover,
			Message:    msg,
			EntityType: "heartbeat",
			EntityID:   h.ID,
			EntityName: h.Name,
			Details:    details,
			Timestamp:  time.Now(),
		})
	})

	// Certificate alerts
	a.certSvc.SetEventCallback(func(eventType string, data interface{}) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]interface{})
		if !ok {
			return
		}

		if eventType == "certificate.alert" || eventType == "certificate.recovery" {
			a.statusSvc.NotifyMonitorChanged(ctx, "certificate", toInt64(m["monitor_id"]))
		}

		switch eventType {
		case "certificate.alert":
			certAlertType, _ := m["alert_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceCertificate,
				AlertType:  certAlertType,
				Severity:   alert.SeverityCritical,
				Message:    fmt.Sprintf("Certificate alert (%s) for %v:%v", certAlertType, m["hostname"], m["port"]),
				EntityType: "certificate",
				EntityID:   toInt64(m["monitor_id"]),
				EntityName: toString(m["hostname"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		case "certificate.recovery":
			sendAlert(alert.Event{
				Source:     alert.SourceCertificate,
				AlertType:  "expiring",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Certificate renewed for %v", m["hostname"]),
				EntityType: "certificate",
				EntityID:   toInt64(m["monitor_id"]),
				EntityName: toString(m["hostname"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// Resource alerts
	a.resourceSvc.SetEventCallback(func(eventType string, data interface{}) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]interface{})
		if !ok {
			return
		}
		switch eventType {
		case "resource.alert":
			resAlertType, _ := m["alert_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceResource,
				AlertType:  resAlertType + "_threshold",
				Severity:   alert.SeverityWarning,
				Message:    fmt.Sprintf("Resource %s threshold exceeded for container %v", resAlertType, m["container_name"]),
				EntityType: "container",
				EntityID:   toInt64(m["container_id"]),
				EntityName: toString(m["container_name"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		case "resource.recovery":
			recoveredType, _ := m["recovered_type"].(string)
			sendAlert(alert.Event{
				Source:     alert.SourceResource,
				AlertType:  recoveredType + "_threshold",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Resource usage returned to normal for container %v", m["container_name"]),
				EntityType: "container",
				EntityID:   toInt64(m["container_id"]),
				EntityName: toString(m["container_name"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// Security insight alerts
	a.securitySvc.SetAlertCallback(func(containerID int64, containerName string, insights []security.Insight, isRecover bool) {
		if isRecover {
			sendAlert(alert.Event{
				Source:     alert.SourceSecurity,
				AlertType:  alert.AlertTypeDangerousConfig,
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("All security issues resolved for container %s", containerName),
				EntityType: "container",
				EntityID:   containerID,
				EntityName: containerName,
				Details:    map[string]any{},
				Timestamp:  time.Now(),
			})
			return
		}
		hs := security.HighestSeverity(insights)
		sendAlert(alert.Event{
			Source:     alert.SourceSecurity,
			AlertType:  alert.AlertTypeDangerousConfig,
			Severity:   MapSecuritySeverity(hs),
			Message:    security.FormatAlertMessage(insights),
			EntityType: "container",
			EntityID:   containerID,
			EntityName: containerName,
			Details: map[string]any{
				"insight_count":    fmt.Sprintf("%d", len(insights)),
				"highest_severity": hs,
			},
			Timestamp: time.Now(),
		})
	})
	a.securitySvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	})
}

// wireUpdateCallback wires the update service event callback.
func (a *App) wireUpdateCallback() {
	alertCh := a.alertEngine.EventChannel()
	ctx := context.Background()

	sendAlert := func(evt alert.Event) {
		alertCh <- evt
		a.statusSvc.HandleAlertEvent(ctx, evt)
	}

	a.updateSvc.SetEventCallback(func(eventType string, data interface{}) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})

		if eventType == event.UpdateDetected {
			if m, ok := data.(map[string]interface{}); ok {
				severity := alert.SeverityInfo
				if rs, ok := m["risk_score"].(int); ok {
					if rs >= 81 {
						severity = alert.SeverityCritical
					} else if rs >= 61 {
						severity = alert.SeverityWarning
					}
				}

				details := map[string]any{
					"image":       m["image"],
					"current_tag": m["current_tag"],
					"latest_tag":  m["latest_tag"],
					"update_type": m["update_type"],
				}

				if extension.CurrentEdition() == extension.Enterprise {
					if cmd, ok := m["update_command"]; ok {
						details["update_command"] = cmd
					}
					if cmd, ok := m["rollback_command"]; ok {
						details["rollback_command"] = cmd
					}
					if url, ok := m["changelog_url"]; ok {
						details["changelog_url"] = url
					}
					if bc, ok := m["has_breaking_changes"]; ok {
						details["has_breaking_changes"] = bc
					}
				}

				containerName, _ := m["container_name"].(string)
				latestTag, _ := m["latest_tag"].(string)
				sendAlert(alert.Event{
					Source:     "update",
					AlertType:  "update_available",
					Severity:   severity,
					Message:    fmt.Sprintf("Update available for %s: %s", containerName, latestTag),
					EntityType: "container",
					EntityName: containerName,
					Details:    details,
					Timestamp:  time.Now(),
				})
			}
		}
	})
}

// wirePostureCallbacks wires the security posture scoring callbacks.
func (a *App) wirePostureCallbacks() {
	alertCh := a.alertEngine.EventChannel()
	ctx := context.Background()

	sendAlert := func(evt alert.Event) {
		alertCh <- evt
		a.statusSvc.HandleAlertEvent(ctx, evt)
	}

	a.scorer.SetPostureAlertCallback(func(score int, previousScore int, color string, isBreach bool) {
		severity := alert.SeverityWarning
		if score < a.scorer.Threshold()-20 {
			severity = alert.SeverityCritical
		}
		msg := fmt.Sprintf("Infrastructure security score dropped to %d (threshold: %d)", score, a.scorer.Threshold())
		if !isBreach {
			severity = alert.SeverityInfo
			msg = fmt.Sprintf("Infrastructure security score recovered to %d (threshold: %d)", score, a.scorer.Threshold())
		}
		sendAlert(alert.Event{
			Source:     alert.SourceSecurity,
			AlertType:  alert.AlertTypePostureThreshold,
			Severity:   severity,
			IsRecover:  !isBreach,
			Message:    msg,
			EntityType: "infrastructure",
			EntityID:   0,
			EntityName: "infrastructure",
			Details: map[string]any{
				"score":          score,
				"previous_score": previousScore,
				"color":          color,
			},
			Timestamp: time.Now(),
		})
	})
	a.scorer.SetPostureEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	})
}

// wireSwarmCallbacks wires Swarm event callbacks for SSE broadcasting.
func (a *App) wireSwarmCallbacks() {
	if a.swarmEvents == nil {
		return
	}
	a.swarmEvents.SetCallback(func(eventType string, data interface{}) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
	})

	// Wire replica health alerting (available for all editions with Swarm).
	{
		alertCh := a.alertEngine.EventChannel()
		ctx := context.Background()
		a.swarmEvents.SetAlertCallback(func(evt alert.Event) {
			alertCh <- evt
			a.statusSvc.HandleAlertEvent(ctx, evt)
		})
	}

	// Wire node service into event processor and alert pipeline (Enterprise).
	if a.swarmNodeSvc != nil {
		a.swarmEvents.SetNodeService(a.swarmNodeSvc)

		alertCh := a.alertEngine.EventChannel()
		ctx := context.Background()

		sseBroadcast := func(eventType string, data interface{}) {
			a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		}
		alertForward := func(evt alert.Event) {
			alertCh <- evt
			a.statusSvc.HandleAlertEvent(ctx, evt)
		}

		a.swarmNodeSvc.SetEventCallback(sseBroadcast)
		a.swarmNodeSvc.SetAlertCallback(alertForward)

		// Wire crash-loop detector (Enterprise).
		if a.swarmCrashLoop != nil {
			a.swarmCrashLoop.SetEventCallback(sseBroadcast)
			a.swarmCrashLoop.SetAlertCallback(alertForward)
		}

		// Wire update tracker (Enterprise).
		if a.swarmUpdateTracker != nil {
			a.swarmUpdateTracker.SetEventCallback(sseBroadcast)
			a.swarmUpdateTracker.SetAlertCallback(alertForward)
		}

		// Wire replica health checker (Enterprise).
		if a.swarmReplicaChecker != nil {
			a.swarmReplicaChecker.SetEventCallback(sseBroadcast)
			a.swarmReplicaChecker.SetAlertCallback(alertForward)
		}
	}

	// Broadcast initial Swarm status.
	if a.swarmCluster != nil {
		a.broker.Broadcast(v1.SSEEvent{
			Type: event.SwarmStatus,
			Data: map[string]interface{}{
				"active":        true,
				"is_manager":    a.swarmCluster.IsManager,
				"cluster_id":    a.swarmCluster.ID,
				"manager_count": a.swarmCluster.ManagerCount,
				"worker_count":  a.swarmCluster.WorkerCount,
			},
		})
	}
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
