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

	"github.com/kolapsis/maintenant/internal/alert"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
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
	a.containerSvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})

		if eventType == "container.state_changed" || eventType == "container.health_changed" {
			if m, ok := data.(map[string]any); ok {
				a.statusSvc.NotifyMonitorChanged(ctx, "container", toString(m["id"]))
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
			if m, ok := data.(map[string]any); ok {
				sendAlert(alert.Event{
					Source:     alert.SourceContainer,
					AlertType:  "restart_loop",
					Severity:   alert.SeverityInfo,
					IsRecover:  true,
					Message:    fmt.Sprintf("Container %s restart rate returned to normal", toString(m["container_name"])),
					EntityType: "container",
					EntityID:   toString(m["container_id"]),
					EntityName: toString(m["container_name"]),
					Timestamp:  time.Now(),
				})
			}
		case "container.archived":
			if m, ok := data.(map[string]any); ok {
				cid := toString(m["id"])
				a.alertEngine.ResolveByEntity(ctx, "container", cid)
				if err := a.statusCompStore.RemoveDanglingMonitorRefs(ctx, "container", cid); err != nil {
					a.logger.Error("failed to remove dangling container refs from status components", "container_id", cid, "error", err)
				}
			}
		case "container.health_changed":
			m, ok := data.(map[string]any)
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
					EntityID:   toString(m["id"]),
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
					EntityID:   toString(m["id"]),
					Details:    m,
					Timestamp:  time.Now(),
				})
			}
		}
	})

	// Endpoint alerts
	a.endpointSvc.SetAlertCallback(func(ep *endpoint.Endpoint, result endpoint.CheckResult) (string, any) {
		a.statusSvc.NotifyMonitorChanged(ctx, "endpoint", ep.ID)

		epName := ep.ContainerName
		if epName == "" {
			epName = ep.Name
		}

		// Certificate trust is independent of the failure thresholds: a degraded
		// host is answering, so it never trips them. Raise it as its own warning
		// and clear it once the chain verifies. Both directions are idempotent —
		// the engine dedups the alert and ignores a recovery with nothing active.
		switch {
		case result.Degraded:
			sendAlert(alert.Event{
				Source:     alert.SourceEndpoint,
				AlertType:  "certificate_untrusted",
				Severity:   alert.SeverityWarning,
				Message:    fmt.Sprintf("Endpoint %s is reachable but its certificate is not trusted: %s", ep.Target, result.DegradedReason),
				EntityType: "endpoint",
				EntityID:   ep.ID,
				EntityName: epName,
				Details: map[string]any{
					"target": ep.Target,
					"reason": result.DegradedReason,
				},
				Timestamp: result.Timestamp,
			})
		case result.Success:
			sendAlert(alert.Event{
				Source:     alert.SourceEndpoint,
				AlertType:  "certificate_untrusted",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Endpoint %s now presents a trusted certificate", ep.Target),
				EntityType: "endpoint",
				EntityID:   ep.ID,
				EntityName: epName,
				Timestamp:  result.Timestamp,
			})
		}

		al := alertDetector.EvaluateCheckResult(ep, result)
		if al == nil {
			return "", nil
		}
		entityName := al.ContainerName
		if entityName == "" {
			entityName = ep.Name
		}
		if al.Type == "alert" {
			sendAlert(alert.Event{
				Source:     alert.SourceEndpoint,
				AlertType:  "consecutive_failure",
				Severity:   alert.SeverityCritical,
				Message:    fmt.Sprintf("Endpoint %s failed %d consecutive checks", al.Target, al.Failures),
				EntityType: "endpoint",
				EntityID:   al.EndpointID,
				EntityName: entityName,
				Details: map[string]any{
					"target":     al.Target,
					"failures":   al.Failures,
					"threshold":  al.Threshold,
					"last_error": al.LastError,
				},
				Timestamp: al.Timestamp,
			})
			return "endpoint.alert", map[string]any{
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
			EntityName: entityName,
			Details: map[string]any{
				"target":    al.Target,
				"successes": al.Successes,
				"threshold": al.Threshold,
			},
			Timestamp: al.Timestamp,
		})
		return "endpoint.recovery", map[string]any{
			"endpoint_id":           al.EndpointID,
			"container_name":        al.ContainerName,
			"target":                al.Target,
			"consecutive_successes": al.Successes,
			"threshold":             al.Threshold,
			"timestamp":             al.Timestamp,
		}
	})

	// Endpoint removal → certificate monitor cleanup + status component dangling ref cleanup.
	a.endpointSvc.SetEndpointRemovedCallback(func(callCtx context.Context, endpointID string) {
		a.certSvc.DeleteByEndpointID(callCtx, endpointID)
		if err := a.statusCompStore.RemoveDanglingMonitorRefs(callCtx, "endpoint", endpointID); err != nil {
			a.logger.Error("failed to remove dangling endpoint refs from status components", "endpoint_id", endpointID, "error", err)
		}
	})

	// Heartbeat alerts
	a.heartbeatSvc.SetAlertCallback(func(h *heartbeat.Heartbeat, alertType string, details map[string]any) {
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

	// Heartbeat events
	a.heartbeatSvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		if eventType == event.HeartbeatDeleted {
			if m, ok := data.(map[string]any); ok {
				hid := toString(m["heartbeat_id"])
				if err := a.statusCompStore.RemoveDanglingMonitorRefs(ctx, "heartbeat", hid); err != nil {
					a.logger.Error("failed to remove dangling heartbeat refs from status components", "heartbeat_id", hid, "error", err)
				}
			}
		}
	})

	// Certificate alerts
	a.certSvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]any)
		if !ok {
			return
		}

		if eventType == "certificate.alert" || eventType == "certificate.recovery" {
			a.statusSvc.NotifyMonitorChanged(ctx, "certificate", toString(m["monitor_id"]))
		}

		if eventType == event.CertificateDeleted {
			certID := toString(m["monitor_id"])
			if err := a.statusCompStore.RemoveDanglingMonitorRefs(ctx, "certificate", certID); err != nil {
				a.logger.Error("failed to remove dangling certificate refs from status components", "certificate_id", certID, "error", err)
			}
		}

		switch eventType {
		case "certificate.alert":
			certAlertType, _ := m["alert_type"].(string)
			severity := alert.SeverityCritical
			if s, ok := m["severity"].(string); ok && s != "" {
				severity = s
			}
			sendAlert(alert.Event{
				Source:     alert.SourceCertificate,
				AlertType:  certAlertType,
				Severity:   severity,
				Message:    fmt.Sprintf("Certificate alert (%s) for %v:%v", certAlertType, m["hostname"], m["port"]),
				EntityType: "certificate",
				EntityID:   toString(m["monitor_id"]),
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
				EntityID:   toString(m["monitor_id"]),
				EntityName: toString(m["hostname"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// Resource alerts
	a.resourceSvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		m, ok := data.(map[string]any)
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
				EntityID:   toString(m["container_id"]),
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
				EntityID:   toString(m["container_id"]),
				EntityName: toString(m["container_name"]),
				Details:    m,
				Timestamp:  time.Now(),
			})
		}
	})

	// Security insight alerts
	a.securitySvc.SetAlertCallback(func(containerID string, containerName string, insights []security.Insight, isRecover bool) {
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

// updateDetectedAlert builds the alert event for a freshly detected image
// update. EntityID is the container's canonical store PK (uid.Container) so the
// alert links to the container detail panel — and so each container's update is
// its own alert; without a unique id every update shares the dedup key
// {update,update_available,container,""} and collides, mixing one container's
// message with another's entity. Falls back to the external id if the UID is
// unavailable. Severity follows the risk score. The caller sets Timestamp.
func updateDetectedAlert(m map[string]any, withChangelog bool) alert.Event {
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
	// The changelog capability decides whether the richer fields ride along.
	if withChangelog {
		for _, k := range []string{"update_command", "rollback_command", "changelog_url", "has_breaking_changes"} {
			if v, ok := m[k]; ok {
				details[k] = v
			}
		}
	}

	containerID, _ := m["container_id"].(string)
	containerName, _ := m["container_name"].(string)
	latestTag, _ := m["latest_tag"].(string)
	entityID, _ := m["container_uid"].(string)
	if entityID == "" {
		entityID = containerID
	}
	return alert.Event{
		Source:     "update",
		AlertType:  "update_available",
		Severity:   severity,
		Message:    fmt.Sprintf("Update available for %s: %s", containerName, latestTag),
		EntityType: "container",
		EntityID:   entityID,
		EntityName: containerName,
		Details:    details,
	}
}

// updateResolvedAlert builds the recovery event when a container's update is no
// longer pending. EntityID uses the same container UID as updateDetectedAlert so
// the right alert is resolved by dedup key. The caller sets Timestamp.
func updateResolvedAlert(m map[string]any) alert.Event {
	containerID, _ := m["container_id"].(string)
	containerName, _ := m["container_name"].(string)
	entityID, _ := m["container_uid"].(string)
	if entityID == "" {
		entityID = containerID
	}
	return alert.Event{
		Source:     "update",
		AlertType:  "update_available",
		Severity:   alert.SeverityInfo,
		IsRecover:  true,
		Message:    fmt.Sprintf("Update no longer required for %s", containerName),
		EntityType: "container",
		EntityID:   entityID,
		EntityName: containerName,
	}
}

// wireUpdateCallback wires the update service event callback.
func (a *App) wireUpdateCallback() {
	alertCh := a.alertEngine.EventChannel()
	ctx := context.Background()

	sendAlert := func(evt alert.Event) {
		evt.Timestamp = time.Now()
		alertCh <- evt
		a.statusSvc.HandleAlertEvent(ctx, evt)
	}

	a.updateSvc.SetEventCallback(func(eventType string, data any) {
		a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})

		m, ok := data.(map[string]any)
		if !ok {
			return
		}

		switch eventType {
		case event.UpdateResolved:
			if name, _ := m["container_name"].(string); name == "" {
				return
			}
			sendAlert(updateResolvedAlert(m))
		case event.UpdateDetected:
			sendAlert(updateDetectedAlert(m, extension.Allows(extension.CapChangelog)))
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
			EntityID:   "",
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
	a.swarmEvents.SetCallback(func(eventType string, data any) {
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

	// Wire node service into event processor and alert pipeline (Pro).
	if a.swarmNodeSvc != nil {
		a.swarmEvents.SetNodeService(a.swarmNodeSvc)

		alertCh := a.alertEngine.EventChannel()
		ctx := context.Background()

		sseBroadcast := func(eventType string, data any) {
			a.broker.Broadcast(v1.SSEEvent{Type: eventType, Data: data})
		}
		alertForward := func(evt alert.Event) {
			alertCh <- evt
			a.statusSvc.HandleAlertEvent(ctx, evt)
		}

		a.swarmNodeSvc.SetEventCallback(sseBroadcast)
		a.swarmNodeSvc.SetAlertCallback(alertForward)

		// Wire crash-loop detector (Pro).
		if a.swarmCrashLoop != nil {
			a.swarmCrashLoop.SetEventCallback(sseBroadcast)
			a.swarmCrashLoop.SetAlertCallback(alertForward)
		}

		// Wire update tracker (Pro).
		if a.swarmUpdateTracker != nil {
			a.swarmUpdateTracker.SetEventCallback(sseBroadcast)
			a.swarmUpdateTracker.SetAlertCallback(alertForward)
		}

		// Wire replica health checker (Pro).
		if a.swarmReplicaChecker != nil {
			a.swarmReplicaChecker.SetEventCallback(sseBroadcast)
			a.swarmReplicaChecker.SetAlertCallback(alertForward)
		}
	}

	// Broadcast initial Swarm status.
	if a.swarmCluster != nil {
		a.broker.Broadcast(v1.SSEEvent{
			Type: event.SwarmStatus,
			Data: map[string]any{
				"active":        true,
				"is_manager":    a.swarmCluster.IsManager,
				"cluster_id":    a.swarmCluster.ID,
				"manager_count": a.swarmCluster.ManagerCount,
				"worker_count":  a.swarmCluster.WorkerCount,
			},
		})
	}
}

// agentLifecycleEvent builds the alert event for an agent connection-state
// change. A genuine outage (stream drop or stale liveness past the threshold)
// raises a Warning — the severity is owned by the alert engine and surfaced
// verbatim by every consumer (dashboard, Alerts page). Reconnection and
// intentional removal (revoke/delete) emit a recovery that clears any pending
// alert. The caller sets Timestamp.
func agentLifecycleEvent(agentID, name, reason string, connected bool) alert.Event {
	evt := alert.Event{
		Source:     "agent",
		AlertType:  "disconnected",
		EntityType: "agent",
		EntityID:   agentID,
		EntityName: name,
	}
	switch {
	case connected:
		// Reconnection clears a pending disconnect alert (no-op if none).
		evt.Severity = alert.SeverityInfo
		evt.IsRecover = true
		evt.Message = fmt.Sprintf("Agent %s reconnected", name)
	case reason == "revoked" || reason == "deleted":
		// Intentional removal: clear any pending alert, never raise one.
		evt.Severity = alert.SeverityInfo
		evt.IsRecover = true
		evt.Message = fmt.Sprintf("Agent %s %s", name, reason)
	default:
		// stream_ended (drop) or stale (liveness) → genuine outage.
		evt.Severity = alert.SeverityWarning
		evt.Message = fmt.Sprintf("Agent %s disconnected (%s)", name, reason)
		evt.Details = map[string]any{"agent_id": agentID, "reason": reason}
	}
	return evt
}

// wireAgentLifecycleAlerts raises a Warning alert when a remote agent's stream
// drops unexpectedly (network outage, or stale liveness past the threshold) and
// resolves it when the agent reconnects. Intentional removals (revoke/delete)
// and graceful shutdown never page. The agent UUID is the alert entity id.
func (a *App) wireAgentLifecycleAlerts() {
	if a.agentSessions == nil {
		return
	}
	ctx := context.Background()
	alertCh := a.alertEngine.EventChannel()

	a.agentSessions.SetLifecycleAlertHook(func(agentID, reason string, connected bool) {
		// Suppress the disconnect storm when the whole server is going down.
		if a.shuttingDown.Load() {
			return
		}

		name := agentID
		if ag, err := a.agentStore.Get(ctx, agentID); err == nil && ag != nil {
			if ag.Label != "" {
				name = ag.Label
			} else if ag.Hostname != "" {
				name = ag.Hostname
			}
		}

		evt := agentLifecycleEvent(agentID, name, reason, connected)
		evt.Timestamp = time.Now()

		alertCh <- evt
		a.statusSvc.HandleAlertEvent(ctx, evt)
	})
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// wireLicenseSubscriber propagates edition transitions to the escalation
// service. Must be called before licenseMgr.Start so the initial transition is
// never missed.
//
// It compares access to alert_escalation before and after rather than the
// editions themselves: of the six directed transitions between three editions,
// four leave escalation untouched (community↔personal, and either of them to
// itself) and only crossing the Pro boundary flips it. Reasoning in capability
// terms means the matrix never has to be enumerated here.
func (a *App) wireLicenseSubscriber(_ context.Context) {
	if a.licenseMgr == nil {
		return
	}
	required := extension.MinEdition(extension.CapAlertEscalation)

	a.licenseMgr.RegisterEditionChangeCallback(func(ctx context.Context, prev, next extension.Edition) {
		was, now := prev.AtLeast(required), next.AtLeast(required)
		switch {
		case was && !now:
			if err := a.escalationSvc.OnEditionDowngraded(ctx); err != nil {
				a.logger.ErrorContext(ctx, "escalation: OnEditionDowngraded failed",
					"error", err, "from", prev, "to", next)
			}
		case !was && now:
			if err := a.escalationSvc.OnEditionUpgraded(ctx); err != nil {
				a.logger.ErrorContext(ctx, "escalation: OnEditionUpgraded failed",
					"error", err, "from", prev, "to", next)
			}
		}
	})
}
