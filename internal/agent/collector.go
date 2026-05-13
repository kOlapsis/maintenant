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
	"github.com/kolapsis/maintenant/internal/runtime"
)

const (
	resourceSampleInterval = 10 * time.Second
)

// RunCollector starts collecting events from the local runtime and pushing them to stream.
// It blocks until ctx is cancelled or a fatal push error occurs.
func RunCollector(ctx context.Context, id *Identity, rt Runtime, stream *PushStream, logger *slog.Logger) error {
	switch rt {
	case RuntimeDocker, RuntimeSwarm:
		return collectDocker(ctx, id, stream, logger)
	case RuntimeKubernetes:
		// K8s collector is not yet implemented; block until context is cancelled.
		logger.Warn("collector: kubernetes event collection not yet implemented")
		<-ctx.Done()
		return nil
	default:
		return fmt.Errorf("collector: unsupported runtime %q", rt)
	}
}

func collectDocker(ctx context.Context, id *Identity, stream *PushStream, logger *slog.Logger) error {
	rt, err := docker.NewRuntime("", logger)
	if err != nil {
		return fmt.Errorf("collector: create docker runtime: %w", err)
	}
	if err := rt.Connect(ctx); err != nil {
		return fmt.Errorf("collector: connect to docker: %w", err)
	}
	defer rt.Close()

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return watchDockerEvents(gCtx, id, rt, stream, logger) })
	g.Go(func() error { return sampleDockerResources(gCtx, id, rt, stream, logger) })
	return g.Wait()
}

func watchDockerEvents(ctx context.Context, id *Identity, rt *docker.Runtime, stream *PushStream, logger *slog.Logger) error {
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

func sampleDockerResources(ctx context.Context, id *Identity, rt *docker.Runtime, stream *PushStream, logger *slog.Logger) error {
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

func collectResourceSnapshots(ctx context.Context, id *Identity, rt *docker.Runtime, stream *PushStream, logger *slog.Logger) error {
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
