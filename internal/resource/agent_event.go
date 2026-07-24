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
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

// HandleAgentEvent records a resource sample pushed by a remote agent.
// The container must already exist (verified via external_id lookup, so an
// orphan FK is never written); its id is the deterministic uid.Container of the
// reporting agent and the Docker external_id, identical to what the agent mints.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ResourceSample) error {
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
			Timestamp:  time.Now(),
		})
		return nil
	}

	c, err := s.containerSvc.GetContainerByExternalID(ctx, containerExternalID)
	if err != nil || c == nil {
		return err
	}

	snap := &ResourceSnapshot{
		ContainerID:     uid.Container(uid.Agent(agentID), containerExternalID),
		CPUPercent:      ev.GetCpuPercent(),
		MemUsed:         clampInt64(ev.GetMemoryBytes()),
		MemLimit:        clampInt64(ev.GetMemoryLimitBytes()),
		NetRxBytes:      clampInt64(ev.GetNetworkRxBytes()),
		NetTxBytes:      clampInt64(ev.GetNetworkTxBytes()),
		BlockReadBytes:  clampInt64(ev.GetDiskReadBytes()),
		BlockWriteBytes: clampInt64(ev.GetDiskWriteBytes()),
		Timestamp:       time.Now(),
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
