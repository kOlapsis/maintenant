// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
)

// EventCallback is called when a Swarm event produces a domain event.
type EventCallback func(eventType string, data interface{})

// EventProcessor handles Swarm-specific runtime events (service/node type).
type EventProcessor struct {
	discovery *ServiceDiscovery
	nodeSvc   *NodeService
	replicas  *ReplicaHealthChecker
	logger    *slog.Logger
	callback  EventCallback
}

// NewEventProcessor creates a new Swarm event processor.
func NewEventProcessor(discovery *ServiceDiscovery, logger *slog.Logger) *EventProcessor {
	return &EventProcessor{
		discovery: discovery,
		logger:    logger,
	}
}

// SetReplicaChecker hands the services changed by events to the replica health checker.
func (ep *EventProcessor) SetReplicaChecker(rhc *ReplicaHealthChecker) {
	ep.replicas = rhc
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
func (ep *EventProcessor) ProcessEvent(ctx context.Context, evt runtime.RuntimeEvent) {
	switch evt.ResourceType {
	case runtime.ResourceService:
		ep.processServiceEvent(ctx, evt)
	case runtime.ResourceNode:
		ep.processNodeEvent(ctx, evt)
	}
}

func (ep *EventProcessor) processServiceEvent(ctx context.Context, evt runtime.RuntimeEvent) {
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
		ep.observeReplicas(svc)

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
		ep.observeReplicas(svc)

	case "remove":
		ep.logger.Info("Swarm service removed", "service_id", serviceID, "name", evt.Name)
		ep.discovery.RemoveService(serviceID)
		ep.emit("swarm.service_removed", map[string]interface{}{
			"service_id": serviceID,
			"name":       evt.Name,
		})
	}
}

func (ep *EventProcessor) processNodeEvent(ctx context.Context, evt runtime.RuntimeEvent) {
	ep.logger.Debug("Swarm node event", "node_id", evt.ExternalID, "action", evt.Action)

	if ep.nodeSvc != nil {
		if err := ep.nodeSvc.Reconcile(ctx); err != nil {
			ep.logger.Warn("node reconciliation on event failed", "error", err)
		}
	}
}

func (ep *EventProcessor) observeReplicas(svc *SwarmService) {
	if ep.replicas != nil {
		ep.replicas.Observe(svc)
	}
}

func (ep *EventProcessor) emit(eventType string, data interface{}) {
	if ep.callback != nil {
		ep.callback(eventType, data)
	}
}
