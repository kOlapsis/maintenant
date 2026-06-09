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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Once the agent container is persisted, resource samples must resolve it by
// external_id and be stored (the cascade that was broken before the fix). The
// snapshot's container_id is the deterministic uid.Container the agent minted.
func TestHandleAgentEvent_PersistsSnapshotForAgentContainer(t *testing.T) {
	extID := "abc123def4567890"
	wantID := uid.Container(uid.Agent("agent-9"), extID)
	c := &container.Container{ID: wantID, ExternalID: extID, AgentID: "agent-9", Name: "demo"}
	csvc := buildContainerSvc(newMockContainerStore(c))

	rstore := newMockResourceStore()
	svc := newTestService(rstore, csvc, nil)

	err := svc.HandleAgentEvent(context.Background(), "agent-9", &agentpb.ResourceSample{
		ContainerId:      extID,
		CpuPercent:       12.5,
		MemoryBytes:      1000,
		MemoryLimitBytes: 2000,
	})
	require.NoError(t, err)

	require.Len(t, rstore.snapshots, 1, "snapshot must be persisted once the container exists")
	snap := rstore.snapshots[0]
	assert.Equal(t, wantID, snap.ContainerID)
	assert.Equal(t, 12.5, snap.CPUPercent)
	assert.Equal(t, "agent-9", snap.AgentID)
}

func TestHandleAgentEvent_SkipsWhenContainerUnknown(t *testing.T) {
	csvc := buildContainerSvc(newMockContainerStore())
	rstore := newMockResourceStore()
	svc := newTestService(rstore, csvc, nil)

	err := svc.HandleAgentEvent(context.Background(), "agent-9", &agentpb.ResourceSample{
		ContainerId: "unknown", CpuPercent: 5,
	})
	require.NoError(t, err)
	assert.Empty(t, rstore.snapshots, "no snapshot when container not yet known")
}
