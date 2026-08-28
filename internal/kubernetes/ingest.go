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
	"context"
	"fmt"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// TopologyStore persists per-agent Kubernetes topology (SQLite-backed).
type TopologyStore interface {
	ReplaceNamespacesForAgent(ctx context.Context, agentID string, namespaces []string) error
	ReplaceWorkloadsForAgent(ctx context.Context, agentID string, workloads []K8sWorkload) error
	ReplacePodsForAgent(ctx context.Context, agentID string, pods []K8sPod) error
	ReplaceNodesForAgent(ctx context.Context, agentID string, nodes []K8sNode) error
	ReplaceEventsForAgent(ctx context.Context, agentID string, events []K8sEventRef) error
}

// IngestService reconciles a Kubernetes topology snapshot reported by an agent
// (or by the server's own local runtime under the LocalAgent id) into the store.
// It implements agentserver.KubernetesTopologyHandler.
type IngestService struct {
	store     TopologyStore
	broadcast func(eventType string, data any)
	logger    *slog.Logger
}

// NewIngestService wires the Kubernetes topology ingest service.
func NewIngestService(store TopologyStore, logger *slog.Logger) *IngestService {
	if logger == nil {
		logger = slog.Default()
	}
	return &IngestService{store: store, logger: logger}
}

// SetBroadcaster wires an SSE broadcaster. When set, a kubernetes.topology_changed
// event carrying the agent id is emitted after a remote agent's snapshot is
// reconciled, so connected clients scoped to that agent refetch.
func (s *IngestService) SetBroadcaster(fn func(eventType string, data any)) {
	s.broadcast = fn
}

// HandleAgentEvent reconciles a full Kubernetes topology snapshot reported by a
// remote agent over gRPC.
func (s *IngestService) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.KubernetesTopology) error {
	if ev == nil {
		return nil
	}
	if err := s.Reconcile(ctx, agentID, protoToSnapshot(ev)); err != nil {
		return err
	}
	if s.broadcast != nil {
		s.broadcast("kubernetes.topology_changed", map[string]any{"agent_id": agentID})
	}
	return nil
}

// Reconcile hard-reconciles a domain snapshot for agentID into the store. Used
// directly by the server's local-runtime reconcile loop (under LocalAgent) and
// indirectly by HandleAgentEvent for remote agents.
func (s *IngestService) Reconcile(ctx context.Context, agentID string, snap TopologySnapshot) error {
	if err := s.store.ReplaceNamespacesForAgent(ctx, agentID, snap.Namespaces); err != nil {
		return fmt.Errorf("reconcile k8s namespaces: %w", err)
	}
	if err := s.store.ReplaceWorkloadsForAgent(ctx, agentID, snap.Workloads); err != nil {
		return fmt.Errorf("reconcile k8s workloads: %w", err)
	}
	if err := s.store.ReplacePodsForAgent(ctx, agentID, snap.Pods); err != nil {
		return fmt.Errorf("reconcile k8s pods: %w", err)
	}
	if err := s.store.ReplaceNodesForAgent(ctx, agentID, snap.Nodes); err != nil {
		return fmt.Errorf("reconcile k8s nodes: %w", err)
	}
	if err := s.store.ReplaceEventsForAgent(ctx, agentID, snap.Events); err != nil {
		return fmt.Errorf("reconcile k8s events: %w", err)
	}
	return nil
}

func protoToSnapshot(ev *agentpb.KubernetesTopology) TopologySnapshot {
	snap := TopologySnapshot{
		Namespaces: ev.GetNamespaces(),
		Workloads:  make([]K8sWorkload, 0, len(ev.GetWorkloads())),
		Pods:       make([]K8sPod, 0, len(ev.GetPods())),
		Nodes:      make([]K8sNode, 0, len(ev.GetNodes())),
		Events:     make([]K8sEventRef, 0, len(ev.GetEvents())),
	}
	for _, m := range ev.GetWorkloads() {
		snap.Workloads = append(snap.Workloads, protoToWorkload(m))
	}
	for _, m := range ev.GetPods() {
		snap.Pods = append(snap.Pods, protoToPod(m))
	}
	for _, m := range ev.GetNodes() {
		snap.Nodes = append(snap.Nodes, protoToNode(m))
	}
	for _, m := range ev.GetEvents() {
		snap.Events = append(snap.Events, protoToEvent(m))
	}
	return snap
}

func protoToEvent(m *agentpb.K8SEventMsg) K8sEventRef {
	return K8sEventRef{
		K8sEvent: K8sEvent{
			Type:      m.GetType(),
			Reason:    m.GetReason(),
			Message:   m.GetMessage(),
			Source:    m.GetSource(),
			FirstSeen: m.GetFirstSeen().AsTime(),
			LastSeen:  m.GetLastSeen().AsTime(),
			Count:     m.GetCount(),
		},
		InvolvedKind:      m.GetInvolvedKind(),
		InvolvedNamespace: m.GetInvolvedNamespace(),
		InvolvedName:      m.GetInvolvedName(),
	}
}

func protoToWorkload(m *agentpb.K8SWorkloadMsg) K8sWorkload {
	return K8sWorkload{
		ID:              m.GetId(),
		Name:            m.GetName(),
		Namespace:       m.GetNamespace(),
		Kind:            m.GetKind(),
		Images:          m.GetImages(),
		ReadyReplicas:   m.GetReadyReplicas(),
		DesiredReplicas: m.GetDesiredReplicas(),
		Status:          m.GetStatus(),
		CreatedAt:       m.GetCreatedAt().AsTime(),
	}
}

func protoToPod(m *agentpb.K8SPodMsg) K8sPod {
	containers := make([]K8sContainerStatus, 0, len(m.GetContainers()))
	for _, c := range m.GetContainers() {
		containers = append(containers, K8sContainerStatus{
			Name:         c.GetName(),
			Image:        c.GetImage(),
			Ready:        c.GetReady(),
			RestartCount: c.GetRestartCount(),
			State:        c.GetState(),
			StateReason:  c.GetStateReason(),
		})
	}
	return K8sPod{
		Name:         m.GetName(),
		Namespace:    m.GetNamespace(),
		Status:       m.GetStatus(),
		StatusReason: m.GetStatusReason(),
		RestartCount: m.GetRestartCount(),
		NodeName:     m.GetNodeName(),
		PodIP:        m.GetPodIp(),
		HostIP:       m.GetHostIp(),
		Containers:   containers,
		WorkloadRef:  m.GetWorkloadRef(),
		CreatedAt:    m.GetCreatedAt().AsTime(),
	}
}

func protoToNode(m *agentpb.K8SNodeMsg) K8sNode {
	return K8sNode{
		Name:              m.GetName(),
		Roles:             m.GetRoles(),
		Status:            m.GetStatus(),
		RunningPods:       int(m.GetRunningPods()),
		KubernetesVersion: m.GetKubernetesVersion(),
		OSImage:           m.GetOsImage(),
		Architecture:      m.GetArchitecture(),
		Capacity: K8sResourceQuantity{
			CPUMillicores: m.GetCapacityCpuMillicores(),
			MemoryBytes:   m.GetCapacityMemoryBytes(),
			Pods:          m.GetCapacityPods(),
		},
		Allocatable: K8sResourceQuantity{
			CPUMillicores: m.GetAllocatableCpuMillicores(),
			MemoryBytes:   m.GetAllocatableMemoryBytes(),
			Pods:          m.GetAllocatablePods(),
		},
		CreatedAt: m.GetCreatedAt().AsTime(),
	}
}
