package resource

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
)

// EventCallback is the function signature for SSE event broadcasting.
type EventCallback func(eventType string, data interface{})

// Service orchestrates resource collection, persistence, and alerting.
type Service struct {
	store         ResourceStore
	containerSvc  *container.Service
	collector     *Collector
	logger        *slog.Logger
	eventCallback EventCallback
}

// NewService creates a resource monitoring service.
func NewService(store ResourceStore, rt pbruntime.Runtime, containerSvc *container.Service, logger *slog.Logger) *Service {
	s := &Service{
		store:        store,
		containerSvc: containerSvc,
		logger:       logger,
	}

	s.collector = NewCollector(rt, containerSvc, logger)
	s.collector.SetOnSnapshot(s.processSnapshot)

	return s
}

// SetEventCallback sets the SSE broadcasting callback.
func (s *Service) SetEventCallback(fn EventCallback) {
	s.eventCallback = fn
}

// Start begins the resource collection loop. Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	s.logger.Info("starting resource collector", "interval", s.collector.interval)
	s.collector.Start(ctx)
}

// GetCurrentSnapshot returns the latest in-memory snapshot for a container.
func (s *Service) GetCurrentSnapshot(containerID int64) *ResourceSnapshot {
	return s.collector.GetLatestSnapshot(containerID)
}

// GetAllLatestSnapshots returns the latest snapshots for all containers.
func (s *Service) GetAllLatestSnapshots() map[int64]*ResourceSnapshot {
	return s.collector.GetAllLatest()
}

// GetContainerName resolves a container ID to its name via the container service.
func (s *Service) GetContainerName(containerID int64) string {
	c, err := s.containerSvc.GetContainer(context.Background(), containerID)
	if err != nil || c == nil {
		return fmt.Sprintf("container-%d", containerID)
	}
	return c.Name
}

// GetHistory returns historical resource snapshots for charting.
func (s *Service) GetHistory(ctx context.Context, containerID int64, timeRange string) ([]*ResourceSnapshot, Granularity, error) {
	now := time.Now()
	var from time.Time
	var granularity Granularity

	switch timeRange {
	case "6h":
		from = now.Add(-6 * time.Hour)
		granularity = Granularity1m
	case "24h":
		from = now.Add(-24 * time.Hour)
		granularity = Granularity5m
	case "7d":
		from = now.Add(-7 * 24 * time.Hour)
		granularity = Granularity1h
	default: // "1h"
		from = now.Add(-1 * time.Hour)
		granularity = GranularityRaw
	}

	snaps, err := s.store.ListSnapshotsAggregated(ctx, containerID, from, now, granularity)
	if err != nil {
		return nil, "", err
	}
	return snaps, granularity, nil
}

// GetAlertConfig returns the alert configuration for a container.
func (s *Service) GetAlertConfig(ctx context.Context, containerID int64) (*ResourceAlertConfig, error) {
	return s.store.GetAlertConfig(ctx, containerID)
}

// UpsertAlertConfig creates or updates alert configuration.
func (s *Service) UpsertAlertConfig(ctx context.Context, cfg *ResourceAlertConfig) error {
	return s.store.UpsertAlertConfig(ctx, cfg)
}

func (s *Service) processSnapshot(snap *ResourceSnapshot) {
	ctx := context.Background()

	if _, err := s.store.InsertSnapshot(ctx, snap); err != nil {
		s.logger.Error("resource: persist snapshot", "container_id", snap.ContainerID, "error", err)
		return
	}

	if s.eventCallback != nil {
		memPercent := 0.0
		if snap.MemLimit > 0 {
			memPercent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
		}
		s.eventCallback("resource.snapshot", map[string]interface{}{
			"container_id":      snap.ContainerID,
			"cpu_percent":       snap.CPUPercent,
			"mem_used":          snap.MemUsed,
			"mem_limit":         snap.MemLimit,
			"mem_percent":       memPercent,
			"net_rx_bytes":      snap.NetRxBytes,
			"net_tx_bytes":      snap.NetTxBytes,
			"block_read_bytes":  snap.BlockReadBytes,
			"block_write_bytes": snap.BlockWriteBytes,
			"timestamp":         snap.Timestamp,
		})
	}

	s.evaluateAlerts(ctx, snap)
}

func (s *Service) evaluateAlerts(ctx context.Context, snap *ResourceSnapshot) {
	cfg, err := s.store.GetAlertConfig(ctx, snap.ContainerID)
	if err != nil {
		s.logger.Error("resource: get alert config", "container_id", snap.ContainerID, "error", err)
		return
	}
	if cfg == nil || !cfg.Enabled {
		return
	}

	memPercent := 0.0
	if snap.MemLimit > 0 {
		memPercent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
	}

	cpuBreaching := snap.CPUPercent >= cfg.CPUThreshold
	memBreaching := memPercent >= cfg.MemThreshold

	if cpuBreaching {
		cfg.CPUConsecutiveBreaches++
	} else {
		cfg.CPUConsecutiveBreaches = 0
	}

	if memBreaching {
		cfg.MemConsecutiveBreaches++
	} else {
		cfg.MemConsecutiveBreaches = 0
	}

	prevState := cfg.AlertState
	var newState AlertState

	cpuAlert := cfg.CPUConsecutiveBreaches >= 2
	memAlert := cfg.MemConsecutiveBreaches >= 2

	switch {
	case cpuAlert && memAlert:
		newState = AlertStateBoth
	case cpuAlert:
		newState = AlertStateCPU
	case memAlert:
		newState = AlertStateMemory
	default:
		newState = AlertStateNormal
	}

	// Determine container name for alert events.
	containerName := ""
	if s.eventCallback != nil && newState != prevState {
		ct, err := s.containerSvc.GetContainer(ctx, snap.ContainerID)
		if err == nil && ct != nil {
			containerName = ct.Name
		}
	}

	now := time.Now()

	// Fire alert events on state transitions.
	if newState != AlertStateNormal && prevState == AlertStateNormal {
		cfg.LastAlertedAt = &now
		if s.eventCallback != nil {
			if cpuAlert {
				s.eventCallback("resource.alert", map[string]interface{}{
					"container_id":   snap.ContainerID,
					"container_name": containerName,
					"alert_type":     "cpu",
					"current_value":  snap.CPUPercent,
					"threshold":      cfg.CPUThreshold,
					"timestamp":      now,
				})
			}
			if memAlert {
				s.eventCallback("resource.alert", map[string]interface{}{
					"container_id":   snap.ContainerID,
					"container_name": containerName,
					"alert_type":     "memory",
					"current_value":  memPercent,
					"threshold":      cfg.MemThreshold,
					"timestamp":      now,
				})
			}
		}
	}

	// Fire recovery event when returning to normal.
	if newState == AlertStateNormal && prevState != AlertStateNormal {
		if s.eventCallback != nil {
			recoveredType := "cpu"
			if prevState == AlertStateMemory {
				recoveredType = "memory"
			} else if prevState == AlertStateBoth {
				recoveredType = "both"
			}
			s.eventCallback("resource.recovery", map[string]interface{}{
				"container_id":   snap.ContainerID,
				"container_name": containerName,
				"recovered_type": recoveredType,
				"current_value":  snap.CPUPercent,
				"threshold":      cfg.CPUThreshold,
				"timestamp":      now,
			})
		}
	}

	cfg.AlertState = newState
	if err := s.store.UpsertAlertConfig(ctx, cfg); err != nil {
		s.logger.Error("resource: update alert config", "container_id", snap.ContainerID, "error", err)
	}
}
