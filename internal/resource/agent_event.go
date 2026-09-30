// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

// HandleAgentEvent records a resource sample pushed by a remote agent.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ResourceSample, meta agentevent.Meta) error {
	containerExternalID := ev.GetContainerId()
	if containerExternalID == "" {
		// Host-level sample: record the agent machine's CPU/mem/disk so the
		// resources view can switch between hosts.
		s.RecordHostSample(&HostSample{
			AgentID:    agentID,
			CPUPercent: ev.GetCpuPercent(),
			MemUsed:    clampInt64(ev.GetMemoryBytes()),
			MemTotal:   clampInt64(ev.GetMemoryLimitBytes()),
			DiskTotal:  ev.GetHostDiskTotalBytes(),
			DiskUsed:   ev.GetHostDiskUsedBytes(),
			Timestamp:  meta.ObservedAt,
			Replayed:   meta.Replayed,
		})
		return nil
	}

	c, err := s.containerSvc.GetContainerByExternalID(ctx, agentID, containerExternalID)
	if err != nil || c == nil || c.IsIgnored {
		return err
	}

	snap := &ResourceSnapshot{
		ContainerID:     c.ID,
		CPUPercent:      ev.GetCpuPercent(),
		MemUsed:         clampInt64(ev.GetMemoryBytes()),
		MemLimit:        clampInt64(ev.GetMemoryLimitBytes()),
		NetRxBytes:      clampInt64(ev.GetNetworkRxBytes()),
		NetTxBytes:      clampInt64(ev.GetNetworkTxBytes()),
		BlockReadBytes:  clampInt64(ev.GetDiskReadBytes()),
		BlockWriteBytes: clampInt64(ev.GetDiskWriteBytes()),
		Timestamp:       meta.ObservedAt,
		Replayed:        meta.Replayed,
		AgentID:         uid.Agent(agentID),
	}
	if meta.EventID != "" {
		snap.ID = uid.EventRecord(snap.AgentID, meta.EventID, "resource_snapshot")
	}

	if !snap.Replayed {
		s.agentLatest.put(snap, time.Now())
	}
	s.processSnapshot(snap)
	return nil
}

const agentSnapshotTTL = 35 * time.Second

// agentLatest ages each sample by its receive time: an agent's skewed clock must not keep a stale sample live.
type agentLatest struct {
	mu      sync.Mutex
	samples map[string]agentSample
}

type agentSample struct {
	snap       *ResourceSnapshot
	receivedAt time.Time
}

func (a *agentLatest) put(snap *ResourceSnapshot, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.samples == nil {
		a.samples = make(map[string]agentSample)
	}
	setNetRates(snap, a.samples[snap.ContainerID].snap)
	a.samples[snap.ContainerID] = agentSample{snap: snap, receivedAt: now}
}

func (a *agentLatest) get(containerID string, now time.Time) *ResourceSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	cur, ok := a.samples[containerID]
	if !ok || now.Sub(cur.receivedAt) > agentSnapshotTTL {
		return nil
	}
	return cur.snap
}

func (a *agentLatest) fresh(now time.Time) map[string]*ResourceSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]*ResourceSnapshot, len(a.samples))
	for id, cur := range a.samples {
		if now.Sub(cur.receivedAt) > agentSnapshotTTL {
			delete(a.samples, id)
			continue
		}
		out[id] = cur.snap
	}
	return out
}

// clampInt64 converts an unsigned byte/count metric to int64, saturating at
// MaxInt64 so an out-of-range value can never wrap to a negative number.
func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
