// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testManager(store model.Store) (*SeriesStateManager, *int64) {
	now := int64(1_700_000_000)
	m := NewSeriesStateManager(store, DefaultConfig(), quietLogger())
	m.now = func() int64 { return now }
	return m, &now
}

func cpuKey() model.SeriesKey {
	return model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricCPU}
}

func TestComputeProgress(t *testing.T) {
	m, _ := testManager(newFakeStore())
	// Time met (14d), coverage half -> 0.5.
	assert.InDelta(t, 0.5, m.ComputeProgress(model.StateLearning,
		Progression{DaysObserved: 14, ActiveBuckets: 100, CoverageFill: 0.5}), 1e-9)
	// Coverage full, time half -> 0.5.
	assert.InDelta(t, 0.5, m.ComputeProgress(model.StateLearning,
		Progression{DaysObserved: 7, ActiveBuckets: 100, CoverageFill: 1.0}), 1e-9)
	// Both full -> 1.0.
	assert.InDelta(t, 1.0, m.ComputeProgress(model.StateLearning,
		Progression{DaysObserved: 28, ActiveBuckets: 168, CoverageFill: 1.0}), 1e-9)
	// No buckets observed -> 0.
	assert.InDelta(t, 0.0, m.ComputeProgress(model.StateLearning,
		Progression{DaysObserved: 28, ActiveBuckets: 0, CoverageFill: 0}), 1e-9)
	// A partial fill (1 of 4 samples per bucket) with no bucket ready still reports progress.
	partial := m.ComputeProgress(model.StateLearning,
		Progression{DaysObserved: 14, ActiveBuckets: 168, ReadyBuckets: 0, CoverageFill: 0.25})
	assert.InDelta(t, 0.25, partial, 1e-9)
	assert.Greater(t, partial, 0.0)
}

func TestEstimatedReadyAt(t *testing.T) {
	cfg := DefaultConfig()
	first := int64(1_000_000)
	// max(required days, min_samples whole weeks): 28 days whether learning or relearning.
	assert.Equal(t, first+int64(28*86400), EstimatedReadyAt(cfg, model.StateLearning, first))
	assert.Equal(t, first+int64(28*86400), EstimatedReadyAt(cfg, model.StateRelearning, first))
}

func TestApplyProgressionLearningToReady(t *testing.T) {
	store := newFakeStore()
	m, _ := testManager(store)
	key := cpuKey()

	// Not enough coverage yet -> stays learning.
	st, changed, err := m.ApplyProgression(context.Background(), key, "node-1", model.SensitivityMedium,
		Progression{FirstSeenAt: 100, DaysObserved: 14, ActiveBuckets: 168, ReadyBuckets: 80, CoverageFill: 0.5})
	require.NoError(t, err)
	assert.Equal(t, model.StateLearning, st.State)
	assert.True(t, changed, "first observation is a change")
	assert.Nil(t, st.ReadyAt)
	assert.Less(t, st.Progress, 1.0)

	// Both criteria met -> ready.
	st, changed, err = m.ApplyProgression(context.Background(), key, "node-1", model.SensitivityMedium,
		Progression{FirstSeenAt: 100, DaysObserved: 28, ActiveBuckets: 168, ReadyBuckets: 168, CoverageFill: 1.0})
	require.NoError(t, err)
	assert.Equal(t, model.StateReady, st.State)
	assert.True(t, changed)
	require.NotNil(t, st.ReadyAt)
	assert.InDelta(t, 1.0, st.Progress, 1e-9)
	assert.Equal(t, int64(100), st.FirstSeenAt, "first_seen is preserved from the first observation")
}

func TestApplyProgressionNeedsBothCriteria(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Progression
	}{
		{"time only", Progression{FirstSeenAt: 1, DaysObserved: 30, ActiveBuckets: 168, ReadyBuckets: 100, CoverageFill: 0.6}},
		{"coverage only", Progression{FirstSeenAt: 1, DaysObserved: 5, ActiveBuckets: 168, ReadyBuckets: 168, CoverageFill: 1.0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			m, _ := testManager(store)
			st, _, err := m.ApplyProgression(context.Background(), cpuKey(), "n", model.SensitivityMedium, tc.p)
			require.NoError(t, err)
			assert.Equal(t, model.StateLearning, st.State, "one criterion alone must not promote to ready")
		})
	}
}

func TestDisableIsSticky(t *testing.T) {
	store := newFakeStore()
	m, _ := testManager(store)
	key := cpuKey()

	changed, err := m.Disable(context.Background(), key)
	require.NoError(t, err)
	assert.True(t, changed)

	// A subsequent recompute that would otherwise promote must not revive it.
	st, changed, err := m.ApplyProgression(context.Background(), key, "n", model.SensitivityMedium,
		Progression{FirstSeenAt: 1, DaysObserved: 28, ActiveBuckets: 168, ReadyBuckets: 168, CoverageFill: 1.0})
	require.NoError(t, err)
	assert.Equal(t, model.StateDisabled, st.State)
	assert.False(t, changed, "disabled series stays disabled and reports no change")
}

func TestResetScopeToRelearning(t *testing.T) {
	store := newFakeStore()
	m, n := testManager(store)
	key := cpuKey()

	// Drive to ready and seed a baseline bucket.
	_, _, err := m.ApplyProgression(context.Background(), key, "n", model.SensitivityMedium,
		Progression{FirstSeenAt: 100, DaysObserved: 28, ActiveBuckets: 168, ReadyBuckets: 168, CoverageFill: 1.0})
	require.NoError(t, err)
	require.NoError(t, store.UpsertBaseline(context.Background(), model.Baseline{SeriesKey: key, Bucket: 3, Median: 5, MAD: 1, SampleCount: 4}))

	*n += 86400
	require.NoError(t, m.ResetScope(context.Background(), key.ScopeType, key.ScopeID, model.ResetReasonImageDigestChange))

	st, err := store.GetSeriesState(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, model.StateRelearning, st.State)
	assert.Nil(t, st.ReadyAt)
	assert.Equal(t, 0, st.ReadyBuckets)
	require.NotNil(t, st.LastResetAt)
	assert.Equal(t, model.ResetReasonImageDigestChange, st.LastResetReason)

	bls, err := store.ListBaselines(context.Background(), key)
	require.NoError(t, err)
	assert.Empty(t, bls, "baselines are purged on reset")
}
