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
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent records a resource sample pushed by a remote agent.
// The container is looked up by its Docker external_id so we can obtain
// the internal container_id FK required by the resource_snapshots table.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ResourceSample) error {
	containerExternalID := ev.GetContainerId()
	if containerExternalID == "" {
		// Host-level sample: record the agent machine's CPU/mem/disk so the
		// resources view can switch between hosts.
		s.RecordHostSample(&HostSample{
			AgentID:    agentID,
			CPUPercent: ev.GetCpuPercent(),
			MemUsed:    int64(ev.GetMemoryBytes()),
			MemTotal:   int64(ev.GetMemoryLimitBytes()),
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
		ContainerID:     c.ID,
		CPUPercent:      ev.GetCpuPercent(),
		MemUsed:         int64(ev.GetMemoryBytes()),
		MemLimit:        int64(ev.GetMemoryLimitBytes()),
		NetRxBytes:      int64(ev.GetNetworkRxBytes()),
		NetTxBytes:      int64(ev.GetNetworkTxBytes()),
		BlockReadBytes:  int64(ev.GetDiskReadBytes()),
		BlockWriteBytes: int64(ev.GetDiskWriteBytes()),
		Timestamp:       time.Now(),
		AgentID:         &agentID,
	}

	s.processSnapshot(snap)
	return nil
}
