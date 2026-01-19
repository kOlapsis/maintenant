package resource

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kolapsis/pulseboard/internal/container"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
)

const (
	defaultCollectInterval = 10 * time.Second
	maxConcurrentStats     = 10
	statsTimeout           = 5 * time.Second
)

// Collector periodically collects resource stats from the active runtime.
type Collector struct {
	rt           pbruntime.Runtime
	containerSvc *container.Service
	interval     time.Duration
	logger       *slog.Logger
	onSnapshot   func(snap *ResourceSnapshot)

	mu     sync.Mutex
	latest map[int64]*ResourceSnapshot // keyed by container_id
}

// NewCollector creates a resource stats collector.
func NewCollector(rt pbruntime.Runtime, containerSvc *container.Service, logger *slog.Logger) *Collector {
	return &Collector{
		rt:           rt,
		containerSvc: containerSvc,
		interval:     defaultCollectInterval,
		logger:       logger,
		latest:       make(map[int64]*ResourceSnapshot),
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
func (c *Collector) GetLatestSnapshot(containerID int64) *ResourceSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest[containerID]
}

// GetAllLatest returns the latest snapshots for all containers.
func (c *Collector) GetAllLatest() map[int64]*ResourceSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[int64]*ResourceSnapshot, len(c.latest))
	for k, v := range c.latest {
		result[k] = v
	}
	return result
}

// Start begins the collection ticker. Blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect once immediately on start.
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

func (c *Collector) collect(ctx context.Context) {
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
				return // first sample, no delta yet
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
	runningIDs := make(map[int64]struct{}, len(running))
	for _, ct := range running {
		runningIDs[ct.ID] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.latest {
		if _, ok := runningIDs[id]; !ok {
			delete(c.latest, id)
		}
	}
}
