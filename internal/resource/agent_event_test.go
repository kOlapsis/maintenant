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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentevent"
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
	}, agentevent.Meta{ObservedAt: time.Now()})
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
	}, agentevent.Meta{ObservedAt: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, rstore.snapshots, "no snapshot when container not yet known")
}

func TestHandleAgentEvent_UsesRowIDNotDerivedID(t *testing.T) {
	extID := "abc123def4567890"
	// An inherited row: its primary key does not derive from its current agent.
	c := &container.Container{ID: uid.New(), ExternalID: extID, AgentID: "agent-9", Name: "demo"}
	require.NotEqual(t, uid.Container(uid.Agent("agent-9"), extID), c.ID)

	rstore := newMockResourceStore()
	svc := newTestService(rstore, buildContainerSvc(newMockContainerStore(c)), nil)

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-9", &agentpb.ResourceSample{
		ContainerId: extID, CpuPercent: 3,
	}, agentevent.Meta{ObservedAt: time.Now()}))
	require.Len(t, rstore.snapshots, 1)
	assert.Equal(t, c.ID, rstore.snapshots[0].ContainerID)
}

func TestHandleAgentEvent_IgnoresOtherAgentsContainer(t *testing.T) {
	extID := "abc123def4567890"
	c := &container.Container{
		ID:         uid.Container(uid.Agent("agent-a"), extID),
		ExternalID: extID, AgentID: "agent-a", Name: "demo",
	}
	rstore := newMockResourceStore()
	svc := newTestService(rstore, buildContainerSvc(newMockContainerStore(c)), nil)

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-b", &agentpb.ResourceSample{
		ContainerId: extID, CpuPercent: 3,
	}, agentevent.Meta{ObservedAt: time.Now()}))
	assert.Empty(t, rstore.snapshots, "a sample must never land on another agent's container")
}

// FR-026: a replayed sample is stored at the time the agent observed it, and it
// must not wake the threshold pipeline.
func TestHandleAgentEvent_ReplayedSampleKeepsObservationTime(t *testing.T) {
	extID := "replay0123456789"
	wantID := uid.Container(uid.Agent("agent-r"), extID)
	c := &container.Container{ID: wantID, ExternalID: extID, AgentID: "agent-r", Name: "demo"}
	csvc := buildContainerSvc(newMockContainerStore(c))

	rstore := newMockResourceStore()
	callbackInvoked := false
	svc := newTestService(rstore, csvc, func(string, interface{}) { callbackInvoked = true })

	observed := time.Now().Add(-90 * time.Minute).Truncate(time.Second)
	err := svc.HandleAgentEvent(context.Background(), "agent-r", &agentpb.ResourceSample{
		ContainerId: extID,
		CpuPercent:  42,
	}, agentevent.Meta{ObservedAt: observed, Replayed: true})
	require.NoError(t, err)

	require.Len(t, rstore.snapshots, 1, "a replayed sample must still be persisted")
	assert.True(t, observed.Equal(rstore.snapshots[0].Timestamp),
		"the snapshot must carry the observation time, not the receive time")
	assert.True(t, rstore.snapshots[0].Replayed)
	assert.False(t, callbackInvoked, "a replayed sample must not feed the threshold pipeline")
}
