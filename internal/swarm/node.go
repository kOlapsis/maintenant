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
)

// NodeStore abstracts persistence for swarm nodes.
type NodeStore interface {
	UpsertNode(ctx context.Context, node *SwarmNode) error
	ListNodes(ctx context.Context) ([]*SwarmNode, error)
	GetNodeByNodeID(ctx context.Context, nodeID string) (*SwarmNode, error)
	UpdateNodeStatus(ctx context.Context, nodeID, status, availability string) error
	UpdateNodeTaskCount(ctx context.Context, nodeID string, count int) error
}

// NodeAlertCallback is called when a node health alert is detected.
type NodeAlertCallback func(evt alert.Event)

// NodeService monitors Swarm node health and detects status transitions.
type NodeService struct {
	client   ServiceClient
	store    NodeStore
	logger   *slog.Logger
	callback EventCallback
	alertCb  NodeAlertCallback
}

// NewNodeService creates a new node health monitoring service.
func NewNodeService(client ServiceClient, store NodeStore, logger *slog.Logger) *NodeService {
	return &NodeService{
		client: client,
		store:  store,
		logger: logger,
	}
}

// SetEventCallback sets the SSE event callback for node status changes.
func (ns *NodeService) SetEventCallback(cb EventCallback) {
	ns.callback = cb
}

// SetAlertCallback sets the alert callback for node health alerts.
func (ns *NodeService) SetAlertCallback(cb NodeAlertCallback) {
	ns.alertCb = cb
}

// Reconcile fetches the current node list from Docker, reconciles with the store,
// and detects status transitions that should trigger alerts.
func (ns *NodeService) Reconcile(ctx context.Context) error {
	nodes, err := ns.client.NodeList(ctx)
	if err != nil {
		return fmt.Errorf("reconcile nodes: %w", err)
	}

	// Build task count per node.
	taskCounts := make(map[string]int)
	tasks, err := ns.client.TaskList(ctx)
	if err != nil {
		ns.logger.Warn("failed to list tasks for node task counts", "error", err)
	} else {
		for _, t := range tasks {
			if t.Status.State == "running" {
				taskCounts[t.NodeID]++
			}
		}
	}

	now := time.Now()
	managerCount := 0
	readyManagerCount := 0

	for _, n := range nodes {
		nodeID := n.ID
		hostname := n.Description.Hostname
		role := string(n.Spec.Role)
		status := string(n.Status.State)
		availability := string(n.Spec.Availability)
		engineVersion := n.Description.Engine.EngineVersion
		address := n.Status.Addr

		if role == "manager" {
			managerCount++
			if status == "ready" {
				readyManagerCount++
			}
		}

		// Look up existing node in store.
		existing, err := ns.store.GetNodeByNodeID(ctx, nodeID)
		if err != nil {
			ns.logger.Error("failed to get node from store", "node_id", nodeID, "error", err)
			continue
		}

		newNode := &SwarmNode{
			NodeID:             nodeID,
			Hostname:           hostname,
			Role:               role,
			Status:             status,
			Availability:       availability,
			EngineVersion:      engineVersion,
			Address:            address,
			TaskCount:          taskCounts[nodeID],
			FirstSeenAt:        now,
			LastSeenAt:         now,
			LastStatusChangeAt: now,
		}

		if existing != nil {
			// Preserve first seen time.
			newNode.FirstSeenAt = existing.FirstSeenAt

			// Detect status transitions.
			ns.detectTransitions(existing, newNode)
		} else {
			ns.logger.Info("new Swarm node discovered",
				"node_id", nodeID, "hostname", hostname, "role", role, "status", status)
		}

		if err := ns.store.UpsertNode(ctx, newNode); err != nil {
			ns.logger.Error("failed to upsert node", "node_id", nodeID, "error", err)
			continue
		}
	}

	// Check quorum degradation for managers.
	if managerCount > 0 {
		ns.checkQuorum(managerCount, readyManagerCount)
	}

	return nil
}

// detectTransitions checks for status changes and emits alerts.
func (ns *NodeService) detectTransitions(old, current *SwarmNode) {
	statusChanged := old.Status != current.Status
	availChanged := old.Availability != current.Availability

	if !statusChanged && !availChanged {
		return
	}

	ns.logger.Info("Swarm node status changed",
		"node_id", current.NodeID,
		"hostname", current.Hostname,
		"old_status", old.Status,
		"new_status", current.Status,
		"old_availability", old.Availability,
		"new_availability", current.Availability,
	)

	// Emit SSE event.
	ns.emit(event.SwarmNodeStatusChanged, map[string]interface{}{
		"node_id":          current.NodeID,
		"hostname":         current.Hostname,
		"role":             current.Role,
		"old_status":       old.Status,
		"new_status":       current.Status,
		"old_availability": old.Availability,
		"new_availability": current.Availability,
	})

	// Node went down or disconnected.
	if statusChanged && old.Status == "ready" && (current.Status == "down" || current.Status == "disconnected") {
		ns.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "node_down",
			Severity:   alert.SeverityCritical,
			Message:    fmt.Sprintf("Swarm node %s (%s) is %s", current.Hostname, current.Role, current.Status),
			EntityType: "swarm_node",
			EntityName: current.Hostname,
			Details: map[string]any{
				"node_id":    current.NodeID,
				"role":       current.Role,
				"old_status": old.Status,
				"new_status": current.Status,
			},
			Timestamp: time.Now(),
		})
	}

	// Node recovered.
	if statusChanged && (old.Status == "down" || old.Status == "disconnected") && current.Status == "ready" {
		ns.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "node_down",
			Severity:   alert.SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Swarm node %s (%s) recovered", current.Hostname, current.Role),
			EntityType: "swarm_node",
			EntityName: current.Hostname,
			Details: map[string]any{
				"node_id":    current.NodeID,
				"role":       current.Role,
				"old_status": old.Status,
				"new_status": current.Status,
			},
			Timestamp: time.Now(),
		})
	}

	// Node drained.
	if availChanged && current.Availability == "drain" && old.Availability != "drain" {
		ns.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "node_drain",
			Severity:   alert.SeverityWarning,
			Message:    fmt.Sprintf("Swarm node %s (%s) set to drain", current.Hostname, current.Role),
			EntityType: "swarm_node",
			EntityName: current.Hostname,
			Details: map[string]any{
				"node_id":          current.NodeID,
				"role":             current.Role,
				"old_availability": old.Availability,
				"new_availability": current.Availability,
			},
			Timestamp: time.Now(),
		})
	}

	// Node un-drained.
	if availChanged && old.Availability == "drain" && current.Availability != "drain" {
		ns.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "node_drain",
			Severity:   alert.SeverityInfo,
			IsRecover:  true,
			Message:    fmt.Sprintf("Swarm node %s (%s) returned to %s", current.Hostname, current.Role, current.Availability),
			EntityType: "swarm_node",
			EntityName: current.Hostname,
			Details: map[string]any{
				"node_id":          current.NodeID,
				"role":             current.Role,
				"old_availability": old.Availability,
				"new_availability": current.Availability,
			},
			Timestamp: time.Now(),
		})
	}
}

// checkQuorum detects when the manager quorum is degraded.
func (ns *NodeService) checkQuorum(totalManagers, readyManagers int) {
	quorumNeeded := (totalManagers / 2) + 1
	if readyManagers < quorumNeeded {
		ns.sendAlert(alert.Event{
			Source:     "swarm",
			AlertType:  "quorum_degraded",
			Severity:   alert.SeverityCritical,
			Message:    fmt.Sprintf("Swarm quorum degraded: %d/%d managers ready (need %d)", readyManagers, totalManagers, quorumNeeded),
			EntityType: "swarm_cluster",
			EntityName: "swarm",
			Details: map[string]any{
				"total_managers": totalManagers,
				"ready_managers": readyManagers,
				"quorum_needed":  quorumNeeded,
			},
			Timestamp: time.Now(),
		})
	}
}

func (ns *NodeService) emit(eventType string, data interface{}) {
	if ns.callback != nil {
		ns.callback(eventType, data)
	}
}

func (ns *NodeService) sendAlert(evt alert.Event) {
	if ns.alertCb != nil {
		ns.alertCb(evt)
	}
}
