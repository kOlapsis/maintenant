// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"testing"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBridge struct {
	opened []*model.AnomalyEvent
	closed []*model.AnomalyEvent
}

func (b *fakeBridge) OnAnomalyOpened(_ context.Context, e *model.AnomalyEvent) (string, error) {
	cp := *e
	b.opened = append(b.opened, &cp)
	return "alert-" + e.ID, nil
}

func (b *fakeBridge) OnAnomalyClosed(_ context.Context, e *model.AnomalyEvent) error {
	cp := *e
	b.closed = append(b.closed, &cp)
	return nil
}

const runnerNow = int64(1_700_000_000)

func runnerCPUKey() model.SeriesKey {
	return model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricCPU}
}

func seedReadyCPU(store *fakeStore, median, mad float64) {
	key := runnerCPUKey()
	_ = store.UpsertSeriesState(context.Background(), &model.SeriesState{
		SeriesKey: key, NodeID: NodeID(""), State: model.StateReady, Sensitivity: model.SensitivityMedium, Progress: 1,
	})
	tow := TimeOfWeekBucket(time.Unix(runnerNow, 0), time.UTC)
	_ = store.UpsertBaseline(context.Background(), model.Baseline{SeriesKey: key, Bucket: tow, Median: median, MAD: mad, SampleCount: 4})
}

func cpuSnap(ts int64, cpu float64) model.SnapshotRow {
	return model.SnapshotRow{
		ScopeIdentity: model.ScopeIdentity{Name: "web-1", RuntimeType: "docker", OrchestrationGroup: "app", OrchestrationUnit: "web"},
		Timestamp:     ts,
		CPUPercent:    cpu,
	}
}

func newRunner(store *fakeStore, bridge AlertBridge) *DetectionRunner {
	r := NewDetectionRunner(store, DefaultConfig(), quietLogger())
	r.now = func() int64 { return runnerNow }
	if bridge != nil {
		r.SetAlertBridge(bridge)
	}
	return r
}

func TestRunnerPassiveBandNoAlert(t *testing.T) {
	store := newFakeStore()
	seedReadyCPU(store, 50, 5) // madEff 5; value 70 -> z≈2.7 -> band
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 70)}
	bridge := &fakeBridge{}
	r := newRunner(store, bridge)

	require.NoError(t, r.Run(context.Background()))

	open, err := store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, model.TierPassive, open.Tier)
	assert.Empty(t, bridge.opened, "a passive band must not raise an alert")
}

func TestRunnerActiveSpikeNeedsPersistence(t *testing.T) {
	store := newFakeStore()
	seedReadyCPU(store, 50, 5) // value 90 -> z≈5.4 -> breach
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 90)}
	bridge := &fakeBridge{}
	r := newRunner(store, bridge)

	// SpikePersistence default 3: first two ticks stay passive, third goes active.
	require.NoError(t, r.Run(context.Background()))
	require.NoError(t, r.Run(context.Background()))
	open, _ := store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	require.NotNil(t, open)
	assert.Equal(t, model.TierPassive, open.Tier, "breach not yet persistent stays passive")
	assert.Empty(t, bridge.opened)

	require.NoError(t, r.Run(context.Background()))
	open, _ = store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	require.NotNil(t, open)
	assert.Equal(t, model.TierActive, open.Tier, "third consecutive breach upgrades to active")
	assert.Len(t, bridge.opened, 1, "active anomaly raises exactly one alert")
}

func TestRunnerResolvesOnReturnToNormal(t *testing.T) {
	store := newFakeStore()
	seedReadyCPU(store, 50, 5)
	bridge := &fakeBridge{}
	r := newRunner(store, bridge)

	// Drive to active.
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 90)}
	for range 3 {
		require.NoError(t, r.Run(context.Background()))
	}
	open, _ := store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	require.NotNil(t, open)
	require.Equal(t, model.TierActive, open.Tier)

	// Return to normal -> event closes and recovery fires.
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 51)}
	require.NoError(t, r.Run(context.Background()))
	open, _ = store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	assert.Nil(t, open, "event closed on return to normal")
	assert.Len(t, bridge.closed, 1, "recovery alert sent on close")
}

func TestRunnerLearningSeriesNotDetected(t *testing.T) {
	store := newFakeStore()
	key := runnerCPUKey()
	// Learning (not ready) series with a baseline and a wildly anomalous value.
	_ = store.UpsertSeriesState(context.Background(), &model.SeriesState{SeriesKey: key, State: model.StateLearning, Sensitivity: model.SensitivityMedium})
	tow := TimeOfWeekBucket(time.Unix(runnerNow, 0), time.UTC)
	_ = store.UpsertBaseline(context.Background(), model.Baseline{SeriesKey: key, Bucket: tow, Median: 50, MAD: 5, SampleCount: 4})
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 200)}
	bridge := &fakeBridge{}
	r := newRunner(store, bridge)

	require.NoError(t, r.Run(context.Background()))
	open, _ := store.GetOpenAnomalyEvent(context.Background(), key, "")
	assert.Nil(t, open, "learning series must not detect")
	assert.Empty(t, bridge.opened, "learning series must not alert")
}

func TestRunnerNoBaselineSkips(t *testing.T) {
	store := newFakeStore()
	key := runnerCPUKey()
	_ = store.UpsertSeriesState(context.Background(), &model.SeriesState{SeriesKey: key, State: model.StateReady, Sensitivity: model.SensitivityMedium})
	// No baseline seeded for the current bucket.
	store.snapshots = []model.SnapshotRow{cpuSnap(runnerNow, 200)}
	r := newRunner(store, nil)
	require.NoError(t, r.Run(context.Background()))
	open, _ := store.GetOpenAnomalyEvent(context.Background(), key, "")
	assert.Nil(t, open)
}

func TestRunnerActiveDrift(t *testing.T) {
	store := newFakeStore()
	seedReadyCPU(store, 50, 5) // madEff 5; drift threshold = 3.5*5 = 17.5
	bridge := &fakeBridge{}
	r := newRunner(store, bridge)

	// Gentle leak whose end-to-end change (20) exceeds the drift threshold but
	// whose latest value (68 -> z≈2.4) is below the spike band.
	leak := []float64{48, 52, 56, 60, 64, 68}
	for i, v := range leak {
		ts := runnerNow - int64((len(leak)-1-i)*sampleAlignSec)
		store.snapshots = append(store.snapshots, cpuSnap(ts, v))
	}
	require.NoError(t, r.Run(context.Background()))

	open, _ := store.GetOpenAnomalyEvent(context.Background(), runnerCPUKey(), "")
	require.NotNil(t, open)
	assert.Equal(t, model.DetectorDrift, open.Detector)
	assert.Equal(t, model.TierActive, open.Tier)
	assert.Len(t, bridge.opened, 1)
}
