// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly_test

import (
	"context"
	"testing"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store"
)

func insertSnapshot(t *testing.T, db *store.DB, containerID string, ts int64, cpu float64) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO resource_snapshots (id, container_id, agent_id, cpu_percent, mem_used, mem_limit, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		VALUES (?, ?, '00000000-0000-0000-0000-000000000000', ?, 0, 0, 0, 0, 0, 0, ?)`,
		"snap-"+containerID+"-"+itoa64(ts), containerID, cpu, ts)
	require.NoError(t, err)
}

func clearSnapshots(t *testing.T, db *store.DB, containerID string) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(), `DELETE FROM resource_snapshots WHERE container_id = ?`, containerID)
	require.NoError(t, err)
}

// seedReadyCPUSeries stores a ready cpu series with a baseline for the current hour of the week.
func seedReadyCPUSeries(t *testing.T, as *store.AnomalyStore, scopeID string, median, mad float64) model.SeriesKey {
	t.Helper()
	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scopeID, Metric: model.MetricCPU}
	now := time.Now()
	require.NoError(t, as.UpsertSeriesState(context.Background(), &model.SeriesState{
		SeriesKey: key, NodeID: anomaly.NodeID(""), State: model.StateReady, Sensitivity: model.SensitivityMedium,
		Progress: 1, FirstSeenAt: now.Add(-28 * 24 * time.Hour).Unix(), UpdatedAt: now.Unix(),
	}))
	tow := anomaly.TimeOfWeekBucket(now, time.UTC)
	require.NoError(t, as.UpsertBaseline(context.Background(), model.Baseline{
		SeriesKey: key, Bucket: tow, Median: median, MAD: mad, SampleCount: 4, UpdatedAt: now.Unix(),
	}))
	return key
}

func newDetectionRunner(as model.Store, raised *[]alert.Event, broadcasts *[]string) *anomaly.DetectionRunner {
	cfg := anomaly.DefaultConfig()
	r := anomaly.NewDetectionRunner(as, cfg, testLogger())
	r.SetAlertBridge(anomaly.NewAnomalyAlertBridge(func(e alert.Event) { *raised = append(*raised, e) }, "warning"))
	r.SetBroadcaster(func(eventType string, _ any) { *broadcasts = append(*broadcasts, eventType) })
	return r
}

func TestDetectionIntegrationActiveAlertAndRecovery(t *testing.T) {
	db := openDB(t)
	as := store.NewAnomalyStore(db)
	insertContainer(t, db, "ctr-web", "app", "web")
	key := seedReadyCPUSeries(t, as, "compose/app/web", 50, 5)

	var raised []alert.Event
	var broadcasts []string
	runner := newDetectionRunner(as, &raised, &broadcasts)

	// Sustained spike: a single very high snapshot, persisted across 3 ticks.
	insertSnapshot(t, db, "ctr-web", time.Now().Unix(), 95)
	for range 3 {
		require.NoError(t, runner.Run(context.Background()))
	}

	open, err := as.GetOpenAnomalyEvent(context.Background(), key, "")
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, model.TierActive, open.Tier)
	require.Len(t, raised, 1, "exactly one anomaly alert raised")
	assert.Equal(t, alert.SourceResourceAnomaly, raised[0].Source)
	assert.Equal(t, anomaly.AlertTypeBehavioralAnomaly, raised[0].AlertType)
	assert.Equal(t, anomaly.EntityTypeAnomalySeries, raised[0].EntityType)
	assert.False(t, raised[0].IsRecover)
	assert.Contains(t, broadcasts, event.AnomalyOpened)

	// Return to normal -> close + recovery (same dedup key, IsRecover=true).
	clearSnapshots(t, db, "ctr-web")
	insertSnapshot(t, db, "ctr-web", time.Now().Unix(), 51)
	require.NoError(t, runner.Run(context.Background()))

	open, err = as.GetOpenAnomalyEvent(context.Background(), key, "")
	require.NoError(t, err)
	assert.Nil(t, open, "event closed on return to normal")
	require.Len(t, raised, 2)
	assert.True(t, raised[1].IsRecover)
	assert.Equal(t, raised[0].EntityID, raised[1].EntityID, "recovery reuses the open's dedup key")
	assert.Contains(t, broadcasts, event.AnomalyClosed)
}

func TestDetectionIntegrationPassiveStaysSilent(t *testing.T) {
	db := openDB(t)
	as := store.NewAnomalyStore(db)
	insertContainer(t, db, "ctr-web", "app", "web")
	key := seedReadyCPUSeries(t, as, "compose/app/web", 50, 5)

	var raised []alert.Event
	var broadcasts []string
	runner := newDetectionRunner(as, &raised, &broadcasts)

	// Moderate deviation (z≈2.7) -> passive band: UI/SSE only, never an alert.
	insertSnapshot(t, db, "ctr-web", time.Now().Unix(), 70)
	for range 3 {
		require.NoError(t, runner.Run(context.Background()))
	}

	open, err := as.GetOpenAnomalyEvent(context.Background(), key, "")
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, model.TierPassive, open.Tier)
	assert.Empty(t, raised, "a passive anomaly must never raise an alert")
}

func TestDetectionIntegrationLearningGate(t *testing.T) {
	db := openDB(t)
	as := store.NewAnomalyStore(db)
	insertContainer(t, db, "ctr-web", "app", "web")

	// Learning (not ready) series + baseline + a wildly anomalous value.
	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricCPU}
	now := time.Now()
	require.NoError(t, as.UpsertSeriesState(context.Background(), &model.SeriesState{
		SeriesKey: key, State: model.StateLearning, Sensitivity: model.SensitivityMedium,
		FirstSeenAt: now.Add(-2 * 24 * time.Hour).Unix(), UpdatedAt: now.Unix(),
	}))
	require.NoError(t, as.UpsertBaseline(context.Background(), model.Baseline{
		SeriesKey: key, Bucket: anomaly.TimeOfWeekBucket(now, time.UTC), Median: 50, MAD: 5, SampleCount: 4, UpdatedAt: now.Unix(),
	}))

	var raised []alert.Event
	var broadcasts []string
	runner := newDetectionRunner(as, &raised, &broadcasts)

	insertSnapshot(t, db, "ctr-web", now.Unix(), 300)
	for range 3 {
		require.NoError(t, runner.Run(context.Background()))
	}

	open, err := as.GetOpenAnomalyEvent(context.Background(), key, "")
	require.NoError(t, err)
	assert.Nil(t, open, "a learning series must not detect")
	assert.Empty(t, raised, "a learning series must not alert")
}
