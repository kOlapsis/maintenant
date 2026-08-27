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

package resource

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/uid"
)

// EventCallback is the function signature for SSE event broadcasting.
type EventCallback func(eventType string, data interface{})

// Deps holds all dependencies for the resource Service.
type Deps struct {
	Store         ResourceStore      // required
	Runtime       runtime.Runtime    // required
	ContainerSvc  *container.Service // required
	Logger        *slog.Logger       // required
	EventCallback EventCallback      // optional — nil-safe

	// RawWindow mirrors the raw retention window so the rollup backfills exactly
	// as far as raw samples still exist. Zero uses DefaultSnapshotRetention.
	RawWindow time.Duration
}

// Service orchestrates resource collection, persistence, and alerting.
type Service struct {
	store         ResourceStore
	containerSvc  *container.Service
	collector     *Collector
	logger        *slog.Logger
	eventCallback EventCallback

	// hosts holds the latest host-level sample reported by each remote agent.
	hosts *hostRegistry

	// noAlertConfigLogged tracks container IDs for which we've already logged
	// "alerts not configured" once. Set membership is the only signal — values
	// are unused.
	noAlertConfigLogged sync.Map

	// rawWindow bounds how far back the rollup backfills.
	rawWindow time.Duration
}

// NewService creates a resource monitoring service.
func NewService(d Deps) *Service {
	if d.Store == nil {
		panic("resource.NewService: Store is required")
	}
	if d.Runtime == nil {
		panic("resource.NewService: Runtime is required")
	}
	if d.ContainerSvc == nil {
		panic("resource.NewService: ContainerSvc is required")
	}
	if d.Logger == nil {
		panic("resource.NewService: Logger is required")
	}
	rawWindow := d.RawWindow
	if rawWindow <= 0 {
		rawWindow = DefaultSnapshotRetention
	}
	s := &Service{
		store:         d.Store,
		containerSvc:  d.ContainerSvc,
		logger:        d.Logger,
		eventCallback: d.EventCallback,
		hosts:         newHostRegistry(),
		rawWindow:     rawWindow,
	}

	s.collector = NewCollector(d.Runtime, d.ContainerSvc, d.Logger)
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
	go s.startRollupLoop(ctx)
	s.collector.Start(ctx)
}

// GetCurrentSnapshot returns the latest in-memory snapshot for a container.
func (s *Service) GetCurrentSnapshot(containerID string) *ResourceSnapshot {
	return s.collector.GetLatestSnapshot(containerID)
}

// GetAllLatestSnapshots returns the latest snapshots for all containers.
func (s *Service) GetAllLatestSnapshots() map[string]*ResourceSnapshot {
	return s.collector.GetAllLatest()
}

// GetHostStat returns the host stat reader for CPU and memory.
func (s *Service) GetHostStat() *HostStatReader {
	return s.collector.GetHostStat()
}

// GetContainerName resolves a container ID to its name via the container service.
func (s *Service) GetContainerName(containerID string) string {
	c, err := s.containerSvc.GetContainer(context.Background(), containerID)
	if err != nil || c == nil {
		return fmt.Sprintf("container-%s", containerID)
	}
	return c.Name
}

// GetHistory returns historical resource snapshots for charting.
//
// Ranges up to 24h group raw samples on the fly; 7d and 30d read the hourly
// rollup, which already holds exactly the buckets those ranges display. That is
// what keeps the raw retention window short. See DefaultSnapshotRetention.
//
// Every range reads a table kept strictly longer than the range itself, so a
// retention pass can never amputate a read in progress.
func (s *Service) GetHistory(ctx context.Context, containerID string, timeRange string) ([]*ResourceSnapshot, Granularity, error) {
	now := time.Now()

	switch timeRange {
	case "90d":
		snaps, err := s.store.ListDailyInRange(ctx, containerID, now.AddDate(0, 0, -90), now)
		if err != nil {
			return nil, "", err
		}
		return snaps, Granularity1d, nil
	case "7d", "30d":
		days := 7
		if timeRange == "30d" {
			days = 30
		}
		snaps, err := s.store.ListHourlyInRange(ctx, containerID, now.AddDate(0, 0, -days), now)
		if err != nil {
			return nil, "", err
		}
		return snaps, Granularity1h, nil
	}

	var from time.Time
	var granularity Granularity
	switch timeRange {
	case "6h":
		from = now.Add(-6 * time.Hour)
		granularity = Granularity1m
	case "24h":
		from = now.Add(-24 * time.Hour)
		granularity = Granularity5m
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
func (s *Service) GetAlertConfig(ctx context.Context, containerID string) (*ResourceAlertConfig, error) {
	return s.store.GetAlertConfig(ctx, containerID)
}

// UpsertAlertConfig creates or updates alert configuration.
func (s *Service) UpsertAlertConfig(ctx context.Context, cfg *ResourceAlertConfig) error {
	return s.store.UpsertAlertConfig(ctx, cfg)
}

// TopConsumersNow ranks containers on their latest sample rather than on a
// history window. It is open in every edition: what the tiering caps is how far
// back a history goes, not the live picture.
//
// agentID filters by host with the same convention as the historical ranking.
func (s *Service) TopConsumersNow(metric string, limit int, agentID *string) []TopConsumerRow {
	all := s.GetAllLatestSnapshots()

	rows := make([]TopConsumerRow, 0, len(all))
	for id, snap := range all {
		if !hostMatchesFilter(snap.AgentID, agentID) {
			continue
		}
		var value, percent float64
		switch metric {
		case "cpu":
			value, percent = snap.CPUPercent, snap.CPUPercent
		case "memory":
			value = float64(snap.MemUsed)
			if snap.MemLimit > 0 {
				percent = float64(snap.MemUsed) / float64(snap.MemLimit) * 100.0
			}
		}
		rows = append(rows, TopConsumerRow{ContainerID: id, AvgValue: value, AvgPercent: percent})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].AvgValue > rows[j].AvgValue })

	if limit < len(rows) {
		rows = rows[:limit]
	}
	for i := range rows {
		rows[i].ContainerName = s.GetContainerName(rows[i].ContainerID)
	}
	return rows
}

// hostMatchesFilter reports whether a sample belongs to the host being asked
// for: nil means every host, "" means the local server, anything else is an
// agent id.
func hostMatchesFilter(snapAgent string, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "" {
		return snapAgent == "" || snapAgent == uid.LocalAgent
	}
	return snapAgent == *filter
}

// GetTopConsumersByPeriod returns the top resource consumers averaged over a
// period. agentID filters by host: nil = all hosts, *agentID == "" = the local
// server, *agentID == id = that agent.
func (s *Service) GetTopConsumersByPeriod(ctx context.Context, metric, period string, limit int, agentID *string) ([]TopConsumerRow, error) {
	rows, err := s.store.GetTopConsumersByPeriod(ctx, metric, period, limit, agentID)
	if err != nil {
		return nil, fmt.Errorf("get top consumers by period: %w", err)
	}
	for i := range rows {
		rows[i].ContainerName = s.GetContainerName(rows[i].ContainerID)
	}
	return rows, nil
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
		s.eventCallback(event.ResourceSnapshot, map[string]interface{}{
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
			"agent_id":          snap.AgentID,
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
		if _, alreadyLogged := s.noAlertConfigLogged.LoadOrStore(snap.ContainerID, struct{}{}); !alreadyLogged {
			s.logger.Debug("resource: alerts not configured", "container_id", snap.ContainerID)
		}
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
		s.logger.Debug("resource: breach detected", "container_id", snap.ContainerID, "metric", "cpu", "consecutive", cfg.CPUConsecutiveBreaches, "threshold", cfg.CPUThreshold, "value", snap.CPUPercent)
	} else {
		cfg.CPUConsecutiveBreaches = 0
	}

	if memBreaching {
		cfg.MemConsecutiveBreaches++
		s.logger.Debug("resource: breach detected", "container_id", snap.ContainerID, "metric", "memory", "consecutive", cfg.MemConsecutiveBreaches, "threshold", cfg.MemThreshold, "value", memPercent)
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
				s.eventCallback(event.ResourceAlert, map[string]interface{}{
					"container_id":   snap.ContainerID,
					"container_name": containerName,
					"alert_type":     "cpu",
					"current_value":  snap.CPUPercent,
					"threshold":      cfg.CPUThreshold,
					"timestamp":      now,
				})
			}
			if memAlert {
				s.eventCallback(event.ResourceAlert, map[string]interface{}{
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
			switch prevState {
			case AlertStateMemory:
				recoveredType = "memory"
			case AlertStateBoth:
				recoveredType = "both"
			}
			s.eventCallback(event.ResourceRecovery, map[string]interface{}{
				"container_id":   snap.ContainerID,
				"container_name": containerName,
				"recovered_type": recoveredType,
				"current_value":  snap.CPUPercent,
				"threshold":      cfg.CPUThreshold,
				"timestamp":      now,
			})
		}
	}

	if newState == prevState {
		s.logger.Debug("resource: alert state unchanged", "container_id", snap.ContainerID, "state", string(newState))
	}

	cfg.AlertState = newState
	if err := s.store.UpsertAlertConfig(ctx, cfg); err != nil {
		s.logger.Error("resource: update alert config", "container_id", snap.ContainerID, "error", err)
	}
}
