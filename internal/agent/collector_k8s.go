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

package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/kubernetes"
)

const kubernetesTopologyInterval = 30 * time.Second

// collectKubernetesRuntime runs the two contributions of a Kubernetes agent in
// parallel: the cluster topology stream and the agent host's own resource
// samples. The host samples report CPU/mem/disk of the node the agent pod runs
// on, so the dashboard and cluster overview can show its gauges like any other
// host. Blocks until ctx is cancelled or a push fails.
func collectKubernetesRuntime(ctx context.Context, id *Identity, src kubernetes.SnapshotSource, stream *PushStream, logger *slog.Logger) error {
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return streamKubernetesTopology(gCtx, id, src, stream, logger) })
	g.Go(func() error { return sampleHostResources(gCtx, id, stream, logger) })
	return g.Wait()
}

// streamKubernetesTopology pushes a periodic full Kubernetes topology snapshot
// (namespaces, workloads, pods, nodes) so the server can serve the
// Workloads/Pods/Namespaces/Nodes views for this agent. Sends one snapshot
// immediately, then on each tick. (Per-pod metrics remain a server-side concern.)
func streamKubernetesTopology(ctx context.Context, id *Identity, src kubernetes.SnapshotSource, stream *PushStream, logger *slog.Logger) error {
	send := func() error {
		snap, err := kubernetes.SnapshotFromRuntime(ctx, src)
		if err != nil {
			logger.Warn("collector: kubernetes topology snapshot failed", "err", err)
			return nil
		}
		return stream.Send(kubernetesTopologyEvent(id.AgentID, snap))
	}

	if err := send(); err != nil {
		return err
	}

	ticker := time.NewTicker(kubernetesTopologyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := send(); err != nil {
				return err
			}
		}
	}
}

// kubernetesTopologyEvent marshals a domain snapshot into an AgentEvent. Pure,
// so it is unit-tested without a live cluster or stream.
func kubernetesTopologyEvent(agentID string, snap kubernetes.TopologySnapshot) *agentpb.AgentEvent {
	workloads := make([]*agentpb.K8SWorkloadMsg, 0, len(snap.Workloads))
	for i := range snap.Workloads {
		w := &snap.Workloads[i]
		workloads = append(workloads, &agentpb.K8SWorkloadMsg{
			Id:              w.ID,
			Name:            w.Name,
			Namespace:       w.Namespace,
			Kind:            w.Kind,
			Images:          w.Images,
			ReadyReplicas:   w.ReadyReplicas,
			DesiredReplicas: w.DesiredReplicas,
			Status:          w.Status,
			CreatedAt:       timestamppb.New(w.CreatedAt),
		})
	}

	pods := make([]*agentpb.K8SPodMsg, 0, len(snap.Pods))
	for i := range snap.Pods {
		p := &snap.Pods[i]
		containers := make([]*agentpb.K8SContainerStatusMsg, 0, len(p.Containers))
		for j := range p.Containers {
			c := &p.Containers[j]
			containers = append(containers, &agentpb.K8SContainerStatusMsg{
				Name:         c.Name,
				Image:        c.Image,
				Ready:        c.Ready,
				RestartCount: c.RestartCount,
				State:        c.State,
				StateReason:  c.StateReason,
			})
		}
		pods = append(pods, &agentpb.K8SPodMsg{
			Name:         p.Name,
			Namespace:    p.Namespace,
			Status:       p.Status,
			StatusReason: p.StatusReason,
			RestartCount: p.RestartCount,
			NodeName:     p.NodeName,
			PodIp:        p.PodIP,
			HostIp:       p.HostIP,
			WorkloadRef:  p.WorkloadRef,
			Containers:   containers,
			CreatedAt:    timestamppb.New(p.CreatedAt),
		})
	}

	nodes := make([]*agentpb.K8SNodeMsg, 0, len(snap.Nodes))
	for i := range snap.Nodes {
		n := &snap.Nodes[i]
		nodes = append(nodes, &agentpb.K8SNodeMsg{
			Name:                     n.Name,
			Roles:                    n.Roles,
			Status:                   n.Status,
			RunningPods:              int32(n.RunningPods),
			KubernetesVersion:        n.KubernetesVersion,
			OsImage:                  n.OSImage,
			Architecture:             n.Architecture,
			CapacityCpuMillicores:    n.Capacity.CPUMillicores,
			CapacityMemoryBytes:      n.Capacity.MemoryBytes,
			CapacityPods:             n.Capacity.Pods,
			AllocatableCpuMillicores: n.Allocatable.CPUMillicores,
			AllocatableMemoryBytes:   n.Allocatable.MemoryBytes,
			AllocatablePods:          n.Allocatable.Pods,
			CreatedAt:                timestamppb.New(n.CreatedAt),
		})
	}

	events := make([]*agentpb.K8SEventMsg, 0, len(snap.Events))
	for i := range snap.Events {
		e := &snap.Events[i]
		events = append(events, &agentpb.K8SEventMsg{
			Type:              e.Type,
			Reason:            e.Reason,
			Message:           e.Message,
			Source:            e.Source,
			FirstSeen:         timestamppb.New(e.FirstSeen),
			LastSeen:          timestamppb.New(e.LastSeen),
			Count:             e.Count,
			InvolvedKind:      e.InvolvedKind,
			InvolvedNamespace: e.InvolvedNamespace,
			InvolvedName:      e.InvolvedName,
		})
	}

	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Kubernetes{Kubernetes: &agentpb.KubernetesTopology{
			Namespaces: snap.Namespaces,
			Workloads:  workloads,
			Pods:       pods,
			Nodes:      nodes,
			Events:     events,
		}},
	}
}
