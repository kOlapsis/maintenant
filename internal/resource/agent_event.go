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
	"math"

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
	if err != nil || c == nil {
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

	s.processSnapshot(snap)
	return nil
}

// clampInt64 converts an unsigned byte/count metric to int64, saturating at
// MaxInt64 so an out-of-range value can never wrap to a negative number.
func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
