// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/uid"
)

// A container outside any compose project has NULL orchestration columns; scanning them must not abort the pass.
func TestAnomalyStore_NullOrchestrationColumns(t *testing.T) {
	db := openTestDB(t)
	s := NewAnomalyStore(db)
	ctx := context.Background()
	cid := uid.New()

	_, err := db.Writer().Exec(ctx,
		`INSERT INTO containers (id, agent_id, external_id, name, image, state, orchestration_group, orchestration_unit, runtime_type, first_seen_at, last_state_change_at)
		VALUES (?, ?, 'ext-bare', 'redis', 'redis:7', 'running', NULL, NULL, 'docker', 0, 0)`,
		cid, uid.LocalAgent)
	require.NoError(t, err)
	_, err = db.Writer().Exec(ctx,
		`INSERT INTO resource_snapshots (id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		VALUES (?, ?, ?, 5, 100, 200, 0, 0, 0, 0, 1000)`, uid.New(), cid, uid.LocalAgent)
	require.NoError(t, err)
	_, err = db.Writer().Exec(ctx,
		`INSERT INTO resource_hourly (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, 3600, 5, 100, 200, 0, 0, 6)`, uid.New(), cid)
	require.NoError(t, err)

	snaps, err := s.ListContainerSnapshotsSince(ctx, 0)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.Equal(t, "redis", snaps[0].Name)
	assert.Empty(t, snaps[0].OrchestrationGroup)
	assert.Equal(t, uid.LocalAgent, snaps[0].AgentID)
	assert.InDelta(t, 5.0, snaps[0].CPUPercent, 1e-9)

	hourly, err := s.ListContainerHourly(ctx, 0)
	require.NoError(t, err)
	require.Len(t, hourly, 1)
	assert.Equal(t, "redis", hourly[0].Name)
	assert.Empty(t, hourly[0].OrchestrationUnit)
	assert.Equal(t, 6, hourly[0].SampleCount)
}

func TestAnomalyStore_SettingsUnsetUntilSaved(t *testing.T) {
	s := NewAnomalyStore(openTestDB(t))
	ctx := context.Background()

	got, err := s.GetSettings(ctx)
	require.NoError(t, err)
	assert.Nil(t, got)

	require.NoError(t, s.UpdateSettings(ctx, anomaly.Settings{BucketPull: 5, UpdatedAt: 10}))
	require.NoError(t, s.UpdateSettings(ctx, anomaly.Settings{BucketPull: 8, UpdatedAt: 20}))

	got, err = s.GetSettings(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, anomaly.Settings{BucketPull: 8, UpdatedAt: 20}, *got)
}

func TestAnomalyStore_SeriesStateRoundTrip(t *testing.T) {
	s := NewAnomalyStore(openTestDB(t))
	ctx := context.Background()
	key := anomaly.SeriesKey{ScopeType: anomaly.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: anomaly.MetricCPU}
	readyAt := int64(500)

	require.NoError(t, s.UpsertSeriesState(ctx, &anomaly.SeriesState{
		SeriesKey: key, NodeID: uid.LocalAgent, State: anomaly.StateLearning, Sensitivity: anomaly.SensitivityMedium,
		FirstSeenAt: 100, DaysObserved: 1.5, Progress: 0.25, UpdatedAt: 200,
	}))
	require.NoError(t, s.UpsertSeriesState(ctx, &anomaly.SeriesState{
		SeriesKey: key, NodeID: uid.LocalAgent, State: anomaly.StateReady, Sensitivity: anomaly.SensitivityHigh,
		FirstSeenAt: 100, DaysObserved: 28, Progress: 1, ReadyAt: &readyAt, GlobalMedian: 40, GlobalMAD: 4, UpdatedAt: 600,
	}))

	got, err := s.GetSeriesState(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, anomaly.StateReady, got.State)
	assert.Equal(t, anomaly.SensitivityHigh, got.Sensitivity)
	require.NotNil(t, got.ReadyAt)
	assert.Equal(t, readyAt, *got.ReadyAt)
	assert.Nil(t, got.LastResetAt)
	assert.InDelta(t, 40.0, got.GlobalMedian, 1e-9)

	ready, err := s.ListReadySeriesStates(ctx)
	require.NoError(t, err)
	require.Len(t, ready, 1)

	none, err := s.ListSeriesStates(ctx, anomaly.ScopeTypeHost)
	require.NoError(t, err)
	assert.Empty(t, none)

	missing, err := s.GetSeriesState(ctx, anomaly.SeriesKey{ScopeType: anomaly.ScopeTypeHost, ScopeID: "x", Metric: anomaly.MetricCPU})
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestAnomalyStore_EventsOpenFirstAndUpdated(t *testing.T) {
	s := NewAnomalyStore(openTestDB(t))
	ctx := context.Background()
	key := anomaly.SeriesKey{ScopeType: anomaly.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: anomaly.MetricCPU}

	older := &anomaly.AnomalyEvent{SeriesKey: key, Detector: anomaly.DetectorSpike, Tier: anomaly.TierPassive,
		StartedAt: 100, PeakValue: 80, BaselineMedian: 40, PeakDeviation: 3, CreatedAt: 100}
	closed := &anomaly.AnomalyEvent{SeriesKey: key, Detector: anomaly.DetectorDrift, Tier: anomaly.TierActive,
		StartedAt: 300, PeakValue: 90, BaselineMedian: 40, PeakDeviation: 5, CreatedAt: 300}
	require.NoError(t, s.InsertAnomalyEvent(ctx, older))
	require.NoError(t, s.InsertAnomalyEvent(ctx, closed))
	require.NotEmpty(t, older.ID)
	require.NoError(t, s.CloseAnomalyEvent(ctx, closed.ID, 400))

	older.Tier = anomaly.TierActive
	older.Detector = anomaly.DetectorChangePoint
	older.PeakDeviation = 7
	require.NoError(t, s.UpdateAnomalyEvent(ctx, older))

	open, err := s.GetOpenAnomalyEvent(ctx, key, "")
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, older.ID, open.ID)
	assert.Equal(t, anomaly.TierActive, open.Tier)
	assert.Equal(t, anomaly.DetectorChangePoint, open.Detector)

	byDetector, err := s.GetOpenAnomalyEvent(ctx, key, anomaly.DetectorSpike)
	require.NoError(t, err)
	assert.Nil(t, byDetector)

	all, err := s.ListAnomalyEvents(ctx, anomaly.AnomalyEventFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, older.ID, all[0].ID, "an open event lists before a more recent closed one")
	require.NotNil(t, all[1].EndedAt)
	assert.Equal(t, int64(400), *all[1].EndedAt)

	active := true
	onlyOpen, err := s.ListAnomalyEvents(ctx, anomaly.AnomalyEventFilter{Active: &active, Tier: anomaly.TierActive, Limit: 10})
	require.NoError(t, err)
	require.Len(t, onlyOpen, 1)
	assert.Equal(t, older.ID, onlyOpen[0].ID)
}

func TestAnomalyStore_DeleteBaselinesForScope(t *testing.T) {
	s := NewAnomalyStore(openTestDB(t))
	ctx := context.Background()
	web := anomaly.SeriesKey{ScopeType: anomaly.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: anomaly.MetricCPU}
	db := anomaly.SeriesKey{ScopeType: anomaly.ScopeTypeContainer, ScopeID: "compose/app/db", Metric: anomaly.MetricCPU}

	for _, k := range []anomaly.SeriesKey{web, db} {
		for _, bucket := range []int{3, 1} {
			require.NoError(t, s.UpsertBaseline(ctx, anomaly.Baseline{SeriesKey: k, Bucket: bucket, Median: 10, MAD: 1, SampleCount: 4, UpdatedAt: 1}))
		}
	}
	require.NoError(t, s.UpsertBaseline(ctx, anomaly.Baseline{SeriesKey: web, Bucket: 3, Median: 20, MAD: 2, SampleCount: 5, UpdatedAt: 2}))

	b, err := s.GetBaseline(ctx, web, 3)
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.InDelta(t, 20.0, b.Median, 1e-9)

	list, err := s.ListBaselines(ctx, web)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, 1, list[0].Bucket)

	require.NoError(t, s.DeleteBaselinesForScope(ctx, anomaly.ScopeTypeContainer, "compose/app/web"))
	gone, err := s.ListBaselines(ctx, web)
	require.NoError(t, err)
	assert.Empty(t, gone)
	kept, err := s.ListBaselines(ctx, db)
	require.NoError(t, err)
	assert.Len(t, kept, 2)
}
