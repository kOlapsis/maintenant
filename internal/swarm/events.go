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
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
	pbruntime "github.com/kolapsis/maintenant/internal/runtime"
)

// EventCallback is called when a Swarm event produces a domain event.
type EventCallback func(eventType string, data interface{})

// EventProcessor handles Swarm-specific runtime events (service/node type).
type EventProcessor struct {
	discovery      *ServiceDiscovery
	nodeSvc        *NodeService
	logger         *slog.Logger
	callback       EventCallback
	alertCb        NodeAlertCallback
	replicaAlerted map[string]bool // tracks which services have active replica alerts
}

// NewEventProcessor creates a new Swarm event processor.
func NewEventProcessor(discovery *ServiceDiscovery, logger *slog.Logger) *EventProcessor {
	return &EventProcessor{
		discovery:      discovery,
		logger:         logger,
		replicaAlerted: make(map[string]bool),
	}
}

// SetAlertCallback sets the alert callback for replica health alerts.
func (ep *EventProcessor) SetAlertCallback(cb NodeAlertCallback) {
	ep.alertCb = cb
}

// SetNodeService sets the node service for routing node events.
func (ep *EventProcessor) SetNodeService(ns *NodeService) {
	ep.nodeSvc = ns
}

// SetCallback sets the event callback for broadcasting SSE events.
func (ep *EventProcessor) SetCallback(cb EventCallback) {
	ep.callback = cb
}

// ProcessEvent handles a runtime event of type "service" or "node".
func (ep *EventProcessor) ProcessEvent(ctx context.Context, evt pbruntime.RuntimeEvent) {
	switch evt.ResourceType {
	case pbruntime.ResourceService:
		ep.processServiceEvent(ctx, evt)
	case pbruntime.ResourceNode:
		ep.processNodeEvent(ctx, evt)
	}
}

func (ep *EventProcessor) processServiceEvent(ctx context.Context, evt pbruntime.RuntimeEvent) {
	serviceID := evt.ExternalID

	switch evt.Action {
	case "create":
		ep.logger.Info("Swarm service created", "service_id", serviceID, "name", evt.Name)
		svc, err := ep.discovery.RefreshService(ctx, serviceID)
		if err != nil {
			ep.logger.Warn("failed to fetch new service", "service_id", serviceID, "error", err)
			return
		}
		ep.emit("swarm.service_discovered", map[string]interface{}{
			"service_id":       svc.ServiceID,
			"name":             svc.Name,
			"mode":             svc.Mode,
			"desired_replicas": svc.DesiredReplicas,
			"stack_name":       svc.StackName,
			"image":            svc.Image,
		})

	case "update":
		ep.logger.Debug("Swarm service updated", "service_id", serviceID)
		svc, err := ep.discovery.RefreshService(ctx, serviceID)
		if err != nil {
			ep.logger.Warn("failed to refresh service", "service_id", serviceID, "error", err)
			return
		}
		ep.emit(event.SwarmServiceUpdated, map[string]interface{}{
			"service_id":       svc.ServiceID,
			"name":             svc.Name,
			"desired_replicas": svc.DesiredReplicas,
			"running_replicas": svc.RunningReplicas,
			"image":            svc.Image,
		})
		ep.checkReplicaHealth(svc)

	case "remove":
		ep.logger.Info("Swarm service removed", "service_id", serviceID, "name", evt.Name)
		ep.discovery.RemoveService(serviceID)
		ep.emit("swarm.service_removed", map[string]interface{}{
			"service_id": serviceID,
			"name":       evt.Name,
		})
	}
}

func (ep *EventProcessor) processNodeEvent(ctx context.Context, evt pbruntime.RuntimeEvent) {
	ep.logger.Debug("Swarm node event", "node_id", evt.ExternalID, "action", evt.Action)

	if ep.nodeSvc != nil {
		if err := ep.nodeSvc.Reconcile(ctx); err != nil {
			ep.logger.Warn("node reconciliation on event failed", "error", err)
		}
	}
}

func (ep *EventProcessor) checkReplicaHealth(svc *SwarmService) {
	if svc.Mode != "replicated" || svc.DesiredReplicas == 0 {
		return
	}

	degraded := svc.RunningReplicas < svc.DesiredReplicas
	wasAlerted := ep.replicaAlerted[svc.ServiceID]

	if degraded && !wasAlerted {
		ep.replicaAlerted[svc.ServiceID] = true
		ep.logger.Warn("service replica health degraded",
			"service", svc.Name, "running", svc.RunningReplicas, "desired", svc.DesiredReplicas)

		if ep.alertCb != nil {
			ep.alertCb(alert.Event{
				Source:     "swarm",
				AlertType:  "replica_unhealthy",
				Severity:   alert.SeverityWarning,
				Message:    fmt.Sprintf("Swarm service %s degraded: %d/%d replicas running", svc.Name, svc.RunningReplicas, svc.DesiredReplicas),
				EntityType: "swarm_service",
				EntityName: svc.Name,
				Details: map[string]any{
					"service_id":       svc.ServiceID,
					"running_replicas": svc.RunningReplicas,
					"desired_replicas": svc.DesiredReplicas,
				},
				Timestamp: time.Now(),
			})
		}
	} else if !degraded && wasAlerted {
		delete(ep.replicaAlerted, svc.ServiceID)
		ep.logger.Info("service replica health recovered", "service", svc.Name)

		if ep.alertCb != nil {
			ep.alertCb(alert.Event{
				Source:     "swarm",
				AlertType:  "replica_unhealthy",
				Severity:   alert.SeverityInfo,
				IsRecover:  true,
				Message:    fmt.Sprintf("Swarm service %s recovered: %d/%d replicas running", svc.Name, svc.RunningReplicas, svc.DesiredReplicas),
				EntityType: "swarm_service",
				EntityName: svc.Name,
				Details: map[string]any{
					"service_id":       svc.ServiceID,
					"running_replicas": svc.RunningReplicas,
					"desired_replicas": svc.DesiredReplicas,
				},
				Timestamp: time.Now(),
			})
		}
	}
}

func (ep *EventProcessor) emit(eventType string, data interface{}) {
	if ep.callback != nil {
		ep.callback(eventType, data)
	}
}
