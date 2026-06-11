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

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// ServiceTaskStore persists per-agent swarm services and tasks (SQLite-backed).
type ServiceTaskStore interface {
	ReplaceServicesForAgent(ctx context.Context, agentID string, services []SwarmService) error
	ReplaceTasksForAgent(ctx context.Context, agentID string, tasks []SwarmTask) error
}

// NodeReconciler persists a full per-agent node set (SQLite-backed).
type NodeReconciler interface {
	ReplaceNodesForAgent(ctx context.Context, agentID string, nodes []*SwarmNode) error
}

// IngestService reconciles a swarm topology snapshot reported by an agent (or by
// the server's own local runtime under the LocalAgent id) into the store. It
// implements agentserver.SwarmTopologyHandler.
type IngestService struct {
	store     ServiceTaskStore
	nodes     NodeReconciler
	broadcast func(eventType string, data any)
	logger    *slog.Logger
}

// NewIngestService wires the swarm topology ingest service.
func NewIngestService(store ServiceTaskStore, nodes NodeReconciler, logger *slog.Logger) *IngestService {
	if logger == nil {
		logger = slog.Default()
	}
	return &IngestService{store: store, nodes: nodes, logger: logger}
}

// SetBroadcaster wires an SSE broadcaster. When set, a swarm.topology_changed
// event carrying the agent id is emitted after a remote agent's snapshot is
// reconciled, so connected clients scoped to that agent refetch.
func (s *IngestService) SetBroadcaster(fn func(eventType string, data any)) {
	s.broadcast = fn
}

// HandleAgentEvent reconciles a full swarm topology snapshot reported by a
// remote agent over gRPC.
func (s *IngestService) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.SwarmTopology) error {
	if ev == nil {
		return nil
	}
	if err := s.Reconcile(ctx, agentID, protoToSnapshot(ev)); err != nil {
		return err
	}
	if s.broadcast != nil {
		s.broadcast("swarm.topology_changed", map[string]any{"agent_id": agentID})
	}
	return nil
}

// Reconcile hard-reconciles a domain snapshot for agentID into the store. Used
// directly by the server's local-runtime reconcile loop (under LocalAgent) and
// indirectly by HandleAgentEvent for remote agents.
func (s *IngestService) Reconcile(ctx context.Context, agentID string, snap TopologySnapshot) error {
	if err := s.store.ReplaceServicesForAgent(ctx, agentID, snap.Services); err != nil {
		return fmt.Errorf("reconcile swarm services: %w", err)
	}
	if err := s.store.ReplaceTasksForAgent(ctx, agentID, snap.Tasks); err != nil {
		return fmt.Errorf("reconcile swarm tasks: %w", err)
	}
	nodes := make([]*SwarmNode, len(snap.Nodes))
	for i := range snap.Nodes {
		n := snap.Nodes[i]
		nodes[i] = &n
	}
	if err := s.nodes.ReplaceNodesForAgent(ctx, agentID, nodes); err != nil {
		return fmt.Errorf("reconcile swarm nodes: %w", err)
	}
	return nil
}

// ReconcileServicesTasks reconciles only the services and tasks of a snapshot,
// leaving nodes untouched. Used by the server's local reconcile loop, where the
// Pro NodeService owns the local swarm_nodes rows; reconciling nodes here
// too would let the two writers fight over the same LocalAgent rows.
func (s *IngestService) ReconcileServicesTasks(ctx context.Context, agentID string, snap TopologySnapshot) error {
	if err := s.store.ReplaceServicesForAgent(ctx, agentID, snap.Services); err != nil {
		return fmt.Errorf("reconcile swarm services: %w", err)
	}
	if err := s.store.ReplaceTasksForAgent(ctx, agentID, snap.Tasks); err != nil {
		return fmt.Errorf("reconcile swarm tasks: %w", err)
	}
	return nil
}

func protoToSnapshot(ev *agentpb.SwarmTopology) TopologySnapshot {
	now := time.Now()
	snap := TopologySnapshot{
		Services: make([]SwarmService, 0, len(ev.GetServices())),
		Tasks:    make([]SwarmTask, 0, len(ev.GetTasks())),
		Nodes:    make([]SwarmNode, 0, len(ev.GetNodes())),
	}
	for _, m := range ev.GetServices() {
		snap.Services = append(snap.Services, protoToService(m))
	}
	for _, m := range ev.GetTasks() {
		snap.Tasks = append(snap.Tasks, protoToTask(m))
	}
	for _, m := range ev.GetNodes() {
		snap.Nodes = append(snap.Nodes, *protoToNode(m, now))
	}
	return snap
}

func protoToService(m *agentpb.SwarmServiceMsg) SwarmService {
	return SwarmService{
		ServiceID:       m.GetServiceId(),
		Name:            m.GetName(),
		Image:           m.GetImage(),
		Mode:            m.GetMode(),
		DesiredReplicas: int(m.GetDesiredReplicas()),
		RunningReplicas: int(m.GetRunningReplicas()),
		Labels:          m.GetLabels(),
		StackName:       m.GetStackName(),
		CreatedAt:       m.GetCreatedAt().AsTime(),
	}
}

func protoToTask(m *agentpb.SwarmTaskMsg) SwarmTask {
	t := SwarmTask{
		TaskID:       m.GetTaskId(),
		ServiceID:    m.GetServiceId(),
		NodeID:       m.GetNodeId(),
		Slot:         int(m.GetSlot()),
		State:        m.GetState(),
		DesiredState: m.GetDesiredState(),
		ContainerID:  m.GetContainerId(),
		Error:        m.GetError(),
		Timestamp:    m.GetTimestamp().AsTime(),
		NodeHostname: m.GetNodeHostname(),
	}
	if m.GetHasExitCode() {
		v := int(m.GetExitCode())
		t.ExitCode = &v
	}
	return t
}

func protoToNode(m *agentpb.SwarmNodeMsg, now time.Time) *SwarmNode {
	return &SwarmNode{
		NodeID:             m.GetNodeId(),
		Hostname:           m.GetHostname(),
		Role:               coerceRole(m.GetRole()),
		Status:             coerceNodeStatus(m.GetStatus()),
		Availability:       coerceAvailability(m.GetAvailability()),
		EngineVersion:      m.GetEngineVersion(),
		Address:            m.GetAddress(),
		TaskCount:          int(m.GetTaskCount()),
		FirstSeenAt:        now,
		LastSeenAt:         now,
		LastStatusChangeAt: now,
	}
}

// coerceRole / coerceNodeStatus / coerceAvailability map agent-reported strings
// onto the values the swarm_nodes CHECK constraints accept, defaulting unknown
// inputs to a safe value rather than failing the whole snapshot.
func coerceRole(r string) string {
	if r == "manager" || r == "worker" {
		return r
	}
	return "worker"
}

func coerceNodeStatus(s string) string {
	switch s {
	case "ready", "down", "disconnected", "unknown":
		return s
	default:
		return "unknown"
	}
}

func coerceAvailability(a string) string {
	switch a {
	case "active", "pause", "drain":
		return a
	default:
		return "active"
	}
}
