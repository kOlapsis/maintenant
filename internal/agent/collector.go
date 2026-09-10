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

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	cmodel "github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/hoststat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/swarm"
)

// resourceSampleInterval is the cadence for container and host resource samples.
// A var (not const) so tests can shorten it; never mutated in production.
var resourceSampleInterval = 10 * time.Second

// containerInventoryInterval is the cadence for the full container snapshot,
// aligned with the Swarm and Kubernetes topology snapshots.
var containerInventoryInterval = 30 * time.Second

// RunCollector starts collecting events from the local runtime and pushing them to stream.
// rt is the already-connected runtime resolved by agent.Run; label is the reported
// runtime kind ("docker", "swarm" or "kubernetes").
// Blocks until ctx is cancelled or a fatal push error occurs.
func RunCollector(ctx context.Context, id *Identity, rt runtime.Runtime, label string, spool *Spool, logger *slog.Logger) error {
	switch label {
	case RuntimeDocker, RuntimeSwarm:
		return collectContainerRuntime(ctx, id, rt, label, spool, logger)
	case RuntimeKubernetes:
		src, ok := rt.(kubernetes.SnapshotSource)
		if !ok {
			logger.Warn("collector: kubernetes runtime does not expose a snapshot source; topology disabled")
			<-ctx.Done()
			return nil
		}
		return collectKubernetesRuntime(ctx, id, src, spool, logger)
	default:
		return fmt.Errorf("collector: unsupported runtime %q", label)
	}
}

func collectContainerRuntime(ctx context.Context, id *Identity, rt runtime.Runtime, label string, spool *Spool, logger *slog.Logger) error {
	if err := syncInventory(ctx, id, rt, spool, logger); err != nil {
		return err
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return watchRuntimeEvents(gCtx, id, rt, spool, logger) })
	g.Go(func() error { return streamInventory(gCtx, id, rt, spool, logger) })
	g.Go(func() error { return sampleRuntimeResources(gCtx, id, rt, spool, logger) })
	g.Go(func() error { return sampleHostResources(gCtx, id, spool, logger) })
	g.Go(func() error { return runLabelProbers(gCtx, id, rt, spool, logger) })

	// Swarm: also push a periodic full topology snapshot (services/tasks/nodes)
	// so the server can serve the Services/Tasks/Nodes views for this agent.
	if label == RuntimeSwarm {
		if dr, ok := rt.(*docker.Runtime); ok {
			disc := swarm.NewServiceDiscovery(dr.Client(), logger)
			client := dr.Client()
			g.Go(func() error { return streamSwarmTopology(gCtx, id, disc, client, spool, logger) })
		} else {
			logger.Warn("collector: swarm runtime is not a docker runtime; topology snapshots disabled")
		}
	}

	return g.Wait()
}

// labeledDiscoverer is satisfied by the docker runtime, exposing raw labels so the
// inventory carries compose grouping.
type labeledDiscoverer interface {
	DiscoverAllWithLabels(ctx context.Context) ([]*docker.DiscoveryResult, error)
}

// syncInventory pushes a full snapshot of every container the runtime currently
// knows about, marked complete so the server can reconcile away what it no
// longer sees. Discovery failure yields no message at all.
func syncInventory(ctx context.Context, id *Identity, rt runtime.Runtime, spool *Spool, logger *slog.Logger) error {
	entry := func(c *cmodel.Container, labels map[string]string) *agentpb.ContainerEvent {
		state, ok := containerStateToProto(c.State)
		if !ok {
			return nil
		}
		health := ""
		if c.HealthStatus != nil {
			health = string(*c.HealthStatus)
		}
		return &agentpb.ContainerEvent{
			ContainerId:    c.ExternalID,
			Name:           c.Name,
			Image:          c.Image,
			State:          state,
			Labels:         labels,
			HealthStatus:   health,
			HasHealthCheck: c.HasHealthCheck,
		}
	}

	var entries []*agentpb.ContainerEvent
	if ld, ok := rt.(labeledDiscoverer); ok {
		results, err := ld.DiscoverAllWithLabels(ctx)
		if err != nil {
			logger.Warn("collector: inventory discovery failed", "err", err)
			return nil
		}
		for _, res := range results {
			if e := entry(res.Container, res.Labels); e != nil {
				entries = append(entries, e)
			}
		}
	} else {
		containers, err := rt.DiscoverAll(ctx)
		if err != nil {
			logger.Warn("collector: inventory discovery failed", "err", err)
			return nil
		}
		for _, c := range containers {
			if e := entry(c, nil); e != nil {
				entries = append(entries, e)
			}
		}
	}

	if err := spool.Send(inventoryEvent(id.AgentID, entries)); err != nil {
		logger.Debug("collector: inventory not sent", "error", err)
	}
	return nil
}

func inventoryEvent(agentID string, entries []*agentpb.ContainerEvent) *agentpb.AgentEvent {
	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Inventory{Inventory: &agentpb.ContainerInventory{
			Containers: entries,
			Complete:   true,
		}},
	}
}

// streamInventory resends the full container inventory on a fixed cadence so the
// server can reconcile away containers removed on this host.
func streamInventory(ctx context.Context, id *Identity, rt runtime.Runtime, spool *Spool, logger *slog.Logger) error {
	ticker := time.NewTicker(containerInventoryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := syncInventory(ctx, id, rt, spool, logger); err != nil {
				return err
			}
		}
	}
}

func containerStateToProto(s cmodel.ContainerState) (agentpb.ContainerState, bool) {
	switch s {
	case cmodel.StateRunning:
		return agentpb.ContainerState_CONTAINER_STATE_RUNNING, true
	case cmodel.StateExited, cmodel.StateCompleted:
		return agentpb.ContainerState_CONTAINER_STATE_EXITED, true
	case cmodel.StatePaused:
		return agentpb.ContainerState_CONTAINER_STATE_PAUSED, true
	case cmodel.StateRestarting:
		return agentpb.ContainerState_CONTAINER_STATE_RESTARTING, true
	case cmodel.StateCreated:
		return agentpb.ContainerState_CONTAINER_STATE_CREATED, true
	case cmodel.StateDead:
		return agentpb.ContainerState_CONTAINER_STATE_DEAD, true
	default:
		return agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED, false
	}
}

func watchRuntimeEvents(ctx context.Context, id *Identity, rt runtime.Runtime, spool *Spool, logger *slog.Logger) error {
	evCh := rt.StreamEvents(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-evCh:
			if !ok {
				return nil
			}
			proto := runtimeEventToProto(ev)
			if proto == nil {
				continue
			}
			evt := &agentpb.AgentEvent{
				AgentId:    id.AgentID,
				EventId:    uuid.NewString(),
				ObservedAt: timestamppb.New(ev.Timestamp),
				Body:       &agentpb.AgentEvent_Container{Container: proto},
			}
			if err := spool.Send(evt); err != nil {
				logger.Debug("collector: container event not sent", "error", err)
			}
		}
	}
}

func sampleRuntimeResources(ctx context.Context, id *Identity, rt runtime.Runtime, spool *Spool, logger *slog.Logger) error {
	ticker := time.NewTicker(resourceSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collectResourceSnapshots(ctx, id, rt, spool, logger); err != nil {
				return err
			}
		}
	}
}

// sampleHostResources periodically reports host-level CPU, memory and disk of
// the machine the agent runs on. These samples carry an empty container_id so
// the server routes them to the per-agent host registry. Requires host /proc
// access inside a container (mount -v /proc:/host/proc:ro).
func sampleHostResources(ctx context.Context, id *Identity, spool *Spool, logger *slog.Logger) error {
	reader := hoststat.NewReader()
	// The reader maintains its own 1s sampling loop for accurate CPU deltas.
	go reader.Start(ctx)

	ticker := time.NewTicker(resourceSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			evt := hostResourceEvent(id.AgentID, reader)
			if err := spool.Send(evt); err != nil {
				logger.Debug("collector: host sample not sent", "error", err)
			}
		}
	}
}

// hostResourceEvent builds a host-level AgentEvent from the current reader state.
// Split out so it can be unit-tested without a live stream.
func hostResourceEvent(agentID string, reader *hoststat.Reader) *agentpb.AgentEvent {
	diskTotal, diskUsed := hoststat.DiskUsage("/")
	return &agentpb.AgentEvent{
		AgentId:    agentID,
		EventId:    uuid.NewString(),
		ObservedAt: timestamppb.Now(),
		Body: &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{
			ContainerId:        "", // empty => host-level sample
			CpuPercent:         reader.CPUPercent(),
			MemoryBytes:        clampUint(reader.MemUsed()),
			MemoryLimitBytes:   clampUint(reader.MemTotal()),
			HostDiskTotalBytes: diskTotal,
			HostDiskUsedBytes:  diskUsed,
		}},
	}
}

func collectResourceSnapshots(ctx context.Context, id *Identity, rt runtime.Runtime, spool *Spool, logger *slog.Logger) error {
	containers, err := rt.DiscoverAll(ctx)
	if err != nil {
		logger.Warn("collector: list containers for stats", "err", err)
		return nil
	}

	for _, c := range containers {
		if c.State != cmodel.StateRunning {
			continue
		}
		raw, err := rt.StatsSnapshot(ctx, c.ExternalID)
		if err != nil {
			logger.Debug("collector: stats failed", "container", c.Name, "err", err)
			continue
		}
		if raw == nil {
			// First sample — no delta yet; skip.
			continue
		}

		// Clamp negative values from unavailable metrics.
		netRx := clampUint(raw.NetRxBytes)
		netTx := clampUint(raw.NetTxBytes)
		diskR := clampUint(raw.BlockReadBytes)
		diskW := clampUint(raw.BlockWriteBytes)

		evt := &agentpb.AgentEvent{
			AgentId:    id.AgentID,
			EventId:    uuid.NewString(),
			ObservedAt: timestamppb.New(raw.Timestamp),
			Body: &agentpb.AgentEvent_Resource{Resource: &agentpb.ResourceSample{
				ContainerId:      c.ExternalID,
				CpuPercent:       raw.CPUPercent,
				MemoryBytes:      clampUint(raw.MemUsed),
				MemoryLimitBytes: clampUint(raw.MemLimit),
				NetworkRxBytes:   netRx,
				NetworkTxBytes:   netTx,
				DiskReadBytes:    diskR,
				DiskWriteBytes:   diskW,
			}},
		}
		if err := spool.Send(evt); err != nil {
			logger.Debug("collector: resource sample not sent", "error", err)
		}
	}
	return nil
}

// runtimeEventToProto converts a runtime.RuntimeEvent to a ContainerEvent proto,
// or nil for an action the server has no use for.
func runtimeEventToProto(ev runtime.RuntimeEvent) *agentpb.ContainerEvent {
	switch ev.Action {
	case "destroy":
		// EXITED, not UNSPECIFIED: a server that predates Destroyed maps every
		// event to a state action, and UNSPECIFIED would read as a restart.
		return &agentpb.ContainerEvent{
			ContainerId: ev.ExternalID,
			Name:        ev.Name,
			Image:       ev.Image,
			State:       agentpb.ContainerState_CONTAINER_STATE_EXITED,
			Labels:      ev.Labels,
			Destroyed:   true,
		}
	case "health_status":
		return &agentpb.ContainerEvent{
			ContainerId:    ev.ExternalID,
			Name:           ev.Name,
			Image:          ev.Image,
			State:          agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED,
			Labels:         ev.Labels,
			HealthStatus:   ev.HealthStatus,
			HasHealthCheck: true,
		}
	}

	state, ok := actionToContainerState(ev.Action)
	if !ok {
		return nil
	}
	return &agentpb.ContainerEvent{
		ContainerId:   ev.ExternalID,
		Name:          ev.Name,
		Image:         ev.Image,
		State:         state,
		StatusMessage: ev.ExitCode,
		Labels:        ev.Labels,
	}
}

// actionToContainerState maps a Docker event action to the proto ContainerState.
func actionToContainerState(action string) (agentpb.ContainerState, bool) {
	switch action {
	case "start", "unpause":
		return agentpb.ContainerState_CONTAINER_STATE_RUNNING, true
	case "stop", "die", "kill":
		return agentpb.ContainerState_CONTAINER_STATE_EXITED, true
	case "pause":
		return agentpb.ContainerState_CONTAINER_STATE_PAUSED, true
	case "restart":
		return agentpb.ContainerState_CONTAINER_STATE_RESTARTING, true
	case "create":
		return agentpb.ContainerState_CONTAINER_STATE_CREATED, true
	default:
		return agentpb.ContainerState_CONTAINER_STATE_UNSPECIFIED, false
	}
}

func clampUint(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// clampInt32 narrows a host int counter (replicas, slots, task/pod counts) to
// the int32 proto field, saturating at the int32 bounds instead of wrapping.
func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
