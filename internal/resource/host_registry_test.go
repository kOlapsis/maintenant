// Copyright 2026 Benjamin Touchard (kOlapsis)
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
)

func TestHostRegistry_PutGetList(t *testing.T) {
	r := newHostRegistry()
	assert.Empty(t, r.list())
	assert.Nil(t, r.get("a"))

	r.put(&HostSample{AgentID: "a", CPUPercent: 10})
	r.put(&HostSample{AgentID: "b", CPUPercent: 20})
	r.put(&HostSample{AgentID: "a", CPUPercent: 15}) // overwrite

	assert.Len(t, r.list(), 2)
	got := r.get("a")
	require.NotNil(t, got)
	assert.Equal(t, 15.0, got.CPUPercent)
}

func TestIsHostSampleFresh(t *testing.T) {
	now := time.Now()
	assert.False(t, IsHostSampleFresh(nil, now))
	assert.True(t, IsHostSampleFresh(&HostSample{Timestamp: now}, now))
	assert.True(t, IsHostSampleFresh(&HostSample{Timestamp: now.Add(-HostSampleTTL + time.Second)}, now))
	assert.False(t, IsHostSampleFresh(&HostSample{Timestamp: now.Add(-HostSampleTTL - time.Second)}, now))
}

func TestRecordHostSample_IgnoresEmptyAgentID(t *testing.T) {
	s := &Service{hosts: newHostRegistry()}
	s.RecordHostSample(&HostSample{AgentID: "", CPUPercent: 5})
	s.RecordHostSample(nil)
	assert.Empty(t, s.hosts.list(), "empty/nil host samples must not be stored")

	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 5})
	assert.Len(t, s.hosts.list(), 1)
}

func TestHostStatForAgent_FreshAndStale(t *testing.T) {
	s := &Service{hosts: newHostRegistry()}

	// Fresh sample is returned.
	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 42, Timestamp: time.Now()})
	got := s.HostStatForAgent("agent-1")
	require.NotNil(t, got)
	assert.Equal(t, 42.0, got.CPUPercent)

	// Stale sample is suppressed.
	s.RecordHostSample(&HostSample{AgentID: "agent-2", Timestamp: time.Now().Add(-HostSampleTTL - time.Minute)})
	assert.Nil(t, s.HostStatForAgent("agent-2"))

	// Unknown agent.
	assert.Nil(t, s.HostStatForAgent("nope"))
}

// HandleAgentEvent must route a host-level sample (empty container_id) into the
// host registry rather than treating it as a container snapshot.
func TestHandleAgentEvent_HostLevelSampleRecorded(t *testing.T) {
	rstore := newMockResourceStore()
	csvc := buildContainerSvc(newMockContainerStore())
	svc := newTestService(rstore, csvc, nil)

	err := svc.HandleAgentEvent(context.Background(), "agent-7", &agentpb.ResourceSample{
		ContainerId:        "", // host-level
		CpuPercent:         33.3,
		MemoryBytes:        4_000_000,
		MemoryLimitBytes:   8_000_000,
		HostDiskTotalBytes: 100_000,
		HostDiskUsedBytes:  40_000,
	}, agentevent.Meta{ObservedAt: time.Now()})
	require.NoError(t, err)

	assert.Empty(t, rstore.snapshots, "host-level samples must not become container snapshots")
	got := svc.HostStatForAgent("agent-7")
	require.NotNil(t, got)
	assert.Equal(t, 33.3, got.CPUPercent)
	assert.Equal(t, int64(4_000_000), got.MemUsed)
	assert.Equal(t, int64(8_000_000), got.MemTotal)
	assert.Equal(t, uint64(100_000), got.DiskTotal)
	assert.Equal(t, uint64(40_000), got.DiskUsed)
}

// FR-029: the registry holds a single live sample per agent with no history
// behind it, so a replayed or out of order sample must not evict a fresher one.
func TestRecordHostSample_ReplayedOrStaleDoesNotEvictCurrent(t *testing.T) {
	s := &Service{hosts: newHostRegistry()}
	now := time.Now()

	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 50, Timestamp: now})

	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 5, Timestamp: now.Add(time.Minute), Replayed: true})
	require.NotNil(t, s.hosts.get("agent-1"))
	assert.Equal(t, 50.0, s.hosts.get("agent-1").CPUPercent, "a replayed sample must never land in the registry")

	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 9, Timestamp: now.Add(-time.Hour)})
	assert.Equal(t, 50.0, s.hosts.get("agent-1").CPUPercent, "a sample older than the current one must be dropped")

	s.RecordHostSample(&HostSample{AgentID: "agent-1", CPUPercent: 77, Timestamp: now.Add(time.Second)})
	assert.Equal(t, 77.0, s.hosts.get("agent-1").CPUPercent, "a fresher sample must still replace the current one")
}

// A replayed host sample reaching the resource handler must be dropped by the
// registry rather than dated on reception.
func TestHandleAgentEvent_ReplayedHostSampleIsIgnored(t *testing.T) {
	svc := newTestService(newMockResourceStore(), nil, nil)

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-h", &agentpb.ResourceSample{
		CpuPercent: 11,
	}, agentevent.Meta{ObservedAt: time.Now()}))
	require.NotNil(t, svc.hosts.get("agent-h"))

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "agent-h", &agentpb.ResourceSample{
		CpuPercent: 99,
	}, agentevent.Meta{ObservedAt: time.Now().Add(time.Minute), Replayed: true}))

	assert.Equal(t, 11.0, svc.hosts.get("agent-h").CPUPercent)
}
