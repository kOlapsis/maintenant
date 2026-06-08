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
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agentpb"
	cmodel "github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/hoststat"
	"github.com/kolapsis/maintenant/internal/runtime"
)

const (
	resourceSampleInterval = 10 * time.Second
)

// RunCollector starts collecting events from the local runtime and pushing them to stream.
// rt is the already-connected runtime resolved by agent.Run; label is the reported
// runtime kind ("docker", "swarm" or "kubernetes").
// Blocks until ctx is cancelled or a fatal push error occurs.
func RunCollector(ctx context.Context, id *Identity, rt runtime.Runtime, label string, stream *PushStream, logger *slog.Logger) error {
	switch label {
	case RuntimeDocker, RuntimeSwarm:
		return collectContainerRuntime(ctx, id, rt, stream, logger)
	case RuntimeKubernetes:
		// K8s collector is not yet implemented; block until context is cancelled.
		logger.Warn("collector: kubernetes event collection not yet implemented")
		<-ctx.Done()
		return nil
	default:
		return fmt.Errorf("collector: unsupported runtime %q", label)
	}
}

func collectContainerRuntime(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
	if err := syncInventory(ctx, id, rt, stream, logger); err != nil {
		return err
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return watchRuntimeEvents(gCtx, id, rt, stream, logger) })
	g.Go(func() error { return sampleRuntimeResources(gCtx, id, rt, stream, logger) })
	g.Go(func() error { return sampleHostResources(gCtx, id, stream, logger) })
	return g.Wait()
}

// labeledDiscoverer is satisfied by the docker runtime, exposing raw labels so the
// inventory carries compose grouping.
type labeledDiscoverer interface {
	DiscoverAllWithLabels(ctx context.Context) ([]*docker.DiscoveryResult, error)
}

// syncInventory pushes a container event for every container the runtime currently
// knows about. The live event stream only carries state transitions, so without this
// containers already running at connect time would never reach the server.
func syncInventory(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
	send := func(c *cmodel.Container, labels map[string]string) error {
		state, ok := containerStateToProto(c.State)
		if !ok {
			return nil
		}
		return stream.Send(&agentpb.AgentEvent{
			AgentId:    id.AgentID,
			EventId:    uuid.NewString(),
			ObservedAt: timestamppb.Now(),
			Body: &agentpb.AgentEvent_Container{Container: &agentpb.ContainerEvent{
				ContainerId: c.ExternalID,
				Name:        c.Name,
				Image:       c.Image,
				State:       state,
				Labels:      labels,
			}},
		})
	}

	if ld, ok := rt.(labeledDiscoverer); ok {
		results, err := ld.DiscoverAllWithLabels(ctx)
		if err != nil {
			logger.Warn("collector: inventory discovery failed", "err", err)
			return nil
		}
		for _, res := range results {
			if err := send(res.Container, res.Labels); err != nil {
				return fmt.Errorf("send inventory event: %w", err)
			}
		}
		return nil
	}

	containers, err := rt.DiscoverAll(ctx)
	if err != nil {
		logger.Warn("collector: inventory discovery failed", "err", err)
		return nil
	}
	for _, c := range containers {
		if err := send(c, nil); err != nil {
			return fmt.Errorf("send inventory event: %w", err)
		}
	}
	return nil
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

func watchRuntimeEvents(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
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
			if err := stream.Send(evt); err != nil {
				logger.Debug("collector: send container event failed", "err", err)
				return fmt.Errorf("send container event: %w", err)
			}
		}
	}
}

func sampleRuntimeResources(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
	ticker := time.NewTicker(resourceSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collectResourceSnapshots(ctx, id, rt, stream, logger); err != nil {
				return err
			}
		}
	}
}

// sampleHostResources periodically reports host-level CPU, memory and disk of
// the machine the agent runs on. These samples carry an empty container_id so
// the server routes them to the per-agent host registry. Requires host /proc
// access inside a container (mount -v /proc:/host/proc:ro).
func sampleHostResources(ctx context.Context, id *Identity, stream *PushStream, logger *slog.Logger) error {
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
			if err := stream.Send(evt); err != nil {
				logger.Debug("collector: send host sample failed", "err", err)
				return fmt.Errorf("send host sample: %w", err)
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

func collectResourceSnapshots(ctx context.Context, id *Identity, rt runtime.Runtime, stream *PushStream, logger *slog.Logger) error {
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
				MemoryBytes:      uint64(raw.MemUsed),
				MemoryLimitBytes: uint64(raw.MemLimit),
				NetworkRxBytes:   netRx,
				NetworkTxBytes:   netTx,
				DiskReadBytes:    diskR,
				DiskWriteBytes:   diskW,
			}},
		}
		if err := stream.Send(evt); err != nil {
			logger.Debug("collector: send resource sample failed", "err", err)
			return fmt.Errorf("send resource sample: %w", err)
		}
	}
	return nil
}

// runtimeEventToProto converts a runtime.RuntimeEvent to a ContainerEvent proto.
// Returns nil for event types that should not be pushed (e.g. destroy, health_status).
func runtimeEventToProto(ev runtime.RuntimeEvent) *agentpb.ContainerEvent {
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
