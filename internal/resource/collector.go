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
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/runtime"
	"github.com/kolapsis/maintenant/internal/uid"
)

const (
	defaultCollectInterval = 10 * time.Second
	maxConcurrentStats     = 10
	statsTimeout           = 5 * time.Second
)

// Collector periodically collects resource stats from the active runtime.
type Collector struct {
	rt           runtime.Runtime
	containerSvc *container.Service
	interval     time.Duration
	logger       *slog.Logger
	onSnapshot   func(snap *ResourceSnapshot)

	mu       sync.Mutex
	latest   map[string]*ResourceSnapshot // keyed by container_id
	hostStat *HostStatReader
}

// NewCollector creates a resource stats collector.
func NewCollector(rt runtime.Runtime, containerSvc *container.Service, logger *slog.Logger) *Collector {
	return &Collector{
		rt:           rt,
		containerSvc: containerSvc,
		interval:     defaultCollectInterval,
		logger:       logger,
		latest:       make(map[string]*ResourceSnapshot),
		hostStat:     NewHostStatReader(),
	}
}

// SetInterval overrides the default collection interval.
func (c *Collector) SetInterval(d time.Duration) {
	c.interval = d
}

// SetOnSnapshot sets the callback invoked for each computed snapshot.
func (c *Collector) SetOnSnapshot(fn func(snap *ResourceSnapshot)) {
	c.onSnapshot = fn
}

// GetLatestSnapshot returns the most recent in-memory snapshot for a container.
func (c *Collector) GetLatestSnapshot(containerID string) *ResourceSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest[containerID]
}

// GetAllLatest returns the latest snapshots for all containers.
func (c *Collector) GetAllLatest() map[string]*ResourceSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]*ResourceSnapshot, len(c.latest))
	for k, v := range c.latest {
		result[k] = v
	}
	return result
}

// Start begins the collection ticker. Blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	// Host stats run in their own goroutine at 1s intervals (like htop).
	go c.hostStat.Start(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect container stats once immediately on start.
	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// GetHostStat returns the host stat reader for CPU and memory.
func (c *Collector) GetHostStat() *HostStatReader {
	return c.hostStat
}

func (c *Collector) collect(ctx context.Context) {
	if !c.rt.IsConnected() {
		return
	}
	containers, err := c.containerSvc.ListContainers(ctx, container.ListContainersOpts{})
	if err != nil {
		c.logger.Error("resource collector: list containers", "error", err)
		return
	}

	// Filter running containers only.
	var running []*container.Container
	for _, ct := range containers {
		if ct.State == container.StateRunning {
			running = append(running, ct)
		}
	}

	if len(running) == 0 {
		c.logger.Debug("resource: no running containers, skipping collection")
		return
	}

	// Fan out with bounded concurrency.
	sem := make(chan struct{}, maxConcurrentStats)
	var wg sync.WaitGroup

	for _, ct := range running {
		wg.Add(1)
		go func(ct *container.Container) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			statsCtx, cancel := context.WithTimeout(ctx, statsTimeout)
			defer cancel()

			raw, err := c.rt.StatsSnapshot(statsCtx, ct.ExternalID)
			if err != nil {
				c.logger.Debug("resource collector: stats failed", "container", ct.Name, "error", err)
				return
			}
			if raw == nil {
				c.logger.Debug("resource: first sample, awaiting delta", "container_name", ct.Name)
				return
			}

			snap := &ResourceSnapshot{
				ContainerID:     ct.ID,
				CPUPercent:      raw.CPUPercent,
				MemUsed:         raw.MemUsed,
				MemLimit:        raw.MemLimit,
				NetRxBytes:      raw.NetRxBytes,
				NetTxBytes:      raw.NetTxBytes,
				BlockReadBytes:  raw.BlockReadBytes,
				BlockWriteBytes: raw.BlockWriteBytes,
				Timestamp:       raw.Timestamp,
				AgentID:         uid.LocalAgent,
			}

			c.mu.Lock()
			c.latest[ct.ID] = snap
			c.mu.Unlock()

			if c.onSnapshot != nil {
				c.onSnapshot(snap)
			}
		}(ct)
	}

	wg.Wait()

	// Clean up stale entries for containers no longer running.
	c.cleanStale(running)
}

func (c *Collector) cleanStale(running []*container.Container) {
	runningIDs := make(map[string]struct{}, len(running))
	for _, ct := range running {
		runningIDs[ct.ID] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.latest {
		if _, ok := runningIDs[id]; !ok {
			c.logger.Debug("resource: removed stale snapshot", "container_id", id)
			delete(c.latest, id)
		}
	}
}
