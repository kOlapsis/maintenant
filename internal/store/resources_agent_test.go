// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/uid"
)

// seedHostContainer inserts a container owned by agentID ("" => local server).
func seedHostContainer(t *testing.T, cstore *ContainerStore, extID string, agentID string) string {
	t.Helper()
	now := time.Now()
	c := &container.Container{
		ExternalID:        extID,
		AgentID:           agentID,
		Name:              extID,
		Image:             "img:v1",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	}
	id, err := cstore.InsertContainer(context.Background(), c)
	require.NoError(t, err)
	return id
}

// InsertSnapshot must persist the agent_id column so resource history can be
// scoped per host. The column was previously dropped on every insert.
func TestInsertSnapshot_PersistsAgentID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	agentID := "agent-snap"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: agentID, PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))
	cid := seedHostContainer(t, cstore, "ext-snap", agentID)

	id, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 12.5, MemUsed: 100, MemLimit: 200,
		Timestamp: time.Now(), AgentID: agentID,
	})
	require.NoError(t, err)

	var got string
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT agent_id FROM resource_snapshots WHERE id = ?`, id).Scan(&got))
	assert.Equal(t, agentID, got)
}

func TestInsertSnapshot_SameEventTwiceKeepsOneRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	cid := seedHostContainer(t, cstore, "ext-replay", "")
	id := uid.EventRecord(uid.LocalAgent, "evt-replayed", "resource_snapshot")
	ts := time.Now()

	for _, cpu := range []float64{10, 20} {
		got, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
			ID: id, ContainerID: cid, CPUPercent: cpu, MemUsed: 1, MemLimit: 2, Timestamp: ts,
		})
		require.NoError(t, err)
		assert.Equal(t, id, got)
	}

	var count int
	var cpu float64
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT COUNT(*), MAX(cpu_percent) FROM resource_snapshots WHERE container_id = ?`, cid).Scan(&count, &cpu))
	assert.Equal(t, 1, count)
	assert.EqualValues(t, 20, cpu)
}

func TestInsertTransition_SameEventTwiceKeepsOneRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)

	cid := seedHostContainer(t, cstore, "ext-replay-transition", "")
	id := uid.EventRecord(uid.LocalAgent, "evt-replayed", "state_transition", "ext-replay-transition")

	for range 2 {
		_, err := cstore.InsertTransition(ctx, &container.StateTransition{
			ID: id, ContainerID: cid, PreviousState: container.StateExited,
			NewState: container.StateRunning, Timestamp: time.Now(),
		})
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM state_transitions WHERE container_id = ?`, cid).Scan(&count))
	assert.Equal(t, 1, count)
}

// GetTopConsumersByPeriod must scope by host via the owning container's agent.
func TestGetTopConsumersByPeriod_HostFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	agentID := "agent-top"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: agentID, PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))

	localCID := seedHostContainer(t, cstore, "ext-local", "")
	agentCID := seedHostContainer(t, cstore, "ext-agent", agentID)

	now := time.Now()
	_, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: localCID, CPUPercent: 5, MemUsed: 10, MemLimit: 100, Timestamp: now,
	})
	require.NoError(t, err)
	_, err = rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: agentCID, CPUPercent: 80, MemUsed: 90, MemLimit: 100, Timestamp: now, AgentID: agentID,
	})
	require.NoError(t, err)

	ids := func(rows []resource.TopConsumerRow) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.ContainerID
		}
		return out
	}

	// nil => all hosts.
	all, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{localCID, agentCID}, ids(all))

	// "" => local server only.
	local := ""
	localRows, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, &local)
	require.NoError(t, err)
	assert.Equal(t, []string{localCID}, ids(localRows))

	// specific agent.
	agentRows, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, &agentID)
	require.NoError(t, err)
	assert.Equal(t, []string{agentCID}, ids(agentRows))
}

// The two periods this feature adds to the top consumers, so both endpoints
// accept the same catalogue. 6h reads raw samples like 1h, 90d reads the daily
// rollup like 7d and 30d.
func TestGetTopConsumersByPeriod_AddedWindows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	cid := seedHostContainer(t, cstore, "ext-windows", "")

	// Raw sample three hours back: inside 6h, outside 1h.
	_, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 42, MemUsed: 10, MemLimit: 100,
		Timestamp: time.Now().Add(-3 * time.Hour),
	})
	require.NoError(t, err)

	// Daily bucket sixty days back: inside 90d, outside 30d.
	require.NoError(t, rstore.InsertDailyRollup(ctx, &resource.RollupRow{
		ContainerID:   cid,
		Bucket:        time.Now().UTC().AddDate(0, 0, -60).Truncate(24 * time.Hour),
		AvgCPUPercent: 77,
		AvgMemLimit:   100,
		SampleCount:   24,
	}))

	sixHours, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "6h", 10, nil)
	require.NoError(t, err)
	require.Len(t, sixHours, 1)
	assert.EqualValues(t, 42, sixHours[0].AvgValue)

	oneHour, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, nil)
	require.NoError(t, err)
	assert.Empty(t, oneHour, "the sample is older than an hour")

	ninetyDays, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "90d", 10, nil)
	require.NoError(t, err)
	require.Len(t, ninetyDays, 1)
	assert.EqualValues(t, 77, ninetyDays[0].AvgValue)

	thirtyDays, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "30d", 10, nil)
	require.NoError(t, err)
	assert.Empty(t, thirtyDays, "the bucket is older than thirty days")
}

// A fresh instance has no closed hour or day yet: the rankings over 24 hours and
// more must still show what the samples of the period in progress say.
func TestGetTopConsumersByPeriod_IncludesThePeriodInProgress(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	rstore := NewResourceStore(db)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-fresh", "")

	_, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 40, MemUsed: 25, MemLimit: 100, Timestamp: time.Now(),
	})
	require.NoError(t, err)

	for _, period := range []string{"24h", "7d", "30d", "90d"} {
		cpu, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", period, 10, nil)
		require.NoError(t, err)
		require.Len(t, cpu, 1, period)
		assert.EqualValues(t, 40, cpu[0].AvgValue, period)

		mem, err := rstore.GetTopConsumersByPeriod(ctx, "memory", period, 10, nil)
		require.NoError(t, err)
		require.Len(t, mem, 1, period)
		assert.InDelta(t, 25, mem[0].AvgPercent, 0.001, period)
	}
}

// The period in progress weighs one bucket, like each closed one, and a bucket
// already rolled up is not counted twice.
func TestGetTopConsumersByPeriod_WeighsThePeriodInProgressAsOneBucket(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	rstore := NewResourceStore(db)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-mixed", "")

	now := time.Now().UTC()
	currentHour := now.Truncate(time.Hour)
	for _, cpu := range []float64{40, 60} {
		_, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
			ContainerID: cid, CPUPercent: cpu, MemLimit: 100, Timestamp: now,
		})
		require.NoError(t, err)
	}
	lastHour := currentHour.Add(-time.Hour)
	require.NoError(t, rstore.InsertHourlyRollup(ctx, &resource.RollupRow{
		ContainerID: cid, Bucket: lastHour, AvgCPUPercent: 10, AvgMemLimit: 100, SampleCount: 360,
	}))
	yesterday := startOfUTCDay(now).AddDate(0, 0, -1)
	require.NoError(t, rstore.InsertDailyRollup(ctx, &resource.RollupRow{
		ContainerID: cid, Bucket: yesterday, AvgCPUPercent: 90, AvgMemLimit: 100, SampleCount: 8640,
	}))

	day, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "24h", 10, nil)
	require.NoError(t, err)
	require.Len(t, day, 1)
	assert.InDelta(t, 30, day[0].AvgValue, 0.001, "the closed hour at 10 and the hour in progress at 50")

	today := 50.0
	if !lastHour.Before(startOfUTCDay(now)) {
		today = 30
	}
	week, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "7d", 10, nil)
	require.NoError(t, err)
	require.Len(t, week, 1)
	assert.InDelta(t, (90+today)/2, week[0].AvgValue, 0.001, "yesterday at 90 and today so far")
}

// The host filter applies to the added periods like to the others.
func TestGetTopConsumersByPeriod_AddedWindowsRespectTheHostFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	agentID := "agent-windows"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: agentID, PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))

	localCID := seedHostContainer(t, cstore, "ext-local-w", "")
	agentCID := seedHostContainer(t, cstore, "ext-agent-w", agentID)

	bucket := time.Now().UTC().AddDate(0, 0, -60).Truncate(24 * time.Hour)
	for _, cid := range []string{localCID, agentCID} {
		require.NoError(t, rstore.InsertDailyRollup(ctx, &resource.RollupRow{
			ContainerID: cid, Bucket: bucket, AvgCPUPercent: 50, AvgMemLimit: 100, SampleCount: 24,
		}))
	}

	all, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "90d", 10, nil)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	onlyAgent, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "90d", 10, &agentID)
	require.NoError(t, err)
	require.Len(t, onlyAgent, 1)
	assert.Equal(t, agentCID, onlyAgent[0].ContainerID)
}
