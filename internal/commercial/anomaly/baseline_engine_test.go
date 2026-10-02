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

func newTestEngine(store model.Store, cfg Config, now int64) *BaselineEngine {
	sm := NewSeriesStateManager(store, cfg, quietLogger())
	sm.now = func() int64 { return now }
	e := NewBaselineEngine(store, sm, cfg, quietLogger())
	e.now = func() int64 { return now }
	return e
}

func replicaRow(group, unit, name string, bucket int64, cpu, mem, rx, tx float64) model.HourlyResourceRow {
	return model.HourlyResourceRow{
		ScopeIdentity: model.ScopeIdentity{Name: name, RuntimeType: "docker", OrchestrationGroup: group, OrchestrationUnit: unit},
		Bucket:        bucket, CPUPercent: cpu, MemUsed: mem, NetRxBytes: rx, NetTxBytes: tx, SampleCount: 6,
	}
}

func TestRecomputeAggregatesReplicas(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig()
	cfg.DefaultMetrics = []string{model.MetricCPU, model.MetricMemory, model.MetricNetworkIO}
	now := int64(1_700_000_000)
	// Monday 10:00 UTC bucket; pick any hour, both replicas share it.
	h := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC).Unix()
	tow := TimeOfWeekBucket(time.Unix(h, 0), time.UTC)

	store.hourly = []model.HourlyResourceRow{
		replicaRow("app", "web", "web-1", h, 10, 100, 100, 50),
		replicaRow("app", "web", "web-2", h, 20, 300, 200, 50),
	}

	e := newTestEngine(store, cfg, now)
	require.NoError(t, e.Recompute(context.Background()))

	scope := "compose/app/web"
	cpu, err := store.ListBaselines(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scope, Metric: model.MetricCPU})
	require.NoError(t, err)
	require.Len(t, cpu, 1)
	assert.Equal(t, tow, cpu[0].Bucket)
	assert.InDelta(t, 15.0, cpu[0].Median, 1e-9, "cpu is averaged across replicas")

	mem, err := store.ListBaselines(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scope, Metric: model.MetricMemory})
	require.NoError(t, err)
	require.Len(t, mem, 1)
	assert.InDelta(t, 200.0, mem[0].Median, 1e-9, "memory is averaged across replicas")

	net, err := store.ListBaselines(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: scope, Metric: model.MetricNetworkIO})
	require.NoError(t, err)
	require.Len(t, net, 1)
	assert.InDelta(t, 400.0, net[0].Median, 1e-9, "network_io is summed across replicas (rx+tx, both replicas)")
}

func TestRecomputeReadinessTiming(t *testing.T) {
	now := int64(1_700_000_000)
	cfg := DefaultConfig() // required 14d, window 28d, min_samples 4

	// 28 days of hourly data => every weekly bucket gets exactly 4 samples.
	t.Run("full history -> ready", func(t *testing.T) {
		store := newFakeStore()
		for ts := now - 28*86400; ts < now; ts += 3600 {
			store.hourly = append(store.hourly, replicaRow("app", "api", "api-1", ts, 5, 50, 10, 10))
		}
		e := newTestEngine(store, cfg, now)
		require.NoError(t, e.Recompute(context.Background()))

		st, err := store.GetSeriesState(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/api", Metric: model.MetricCPU})
		require.NoError(t, err)
		require.NotNil(t, st)
		assert.Equal(t, model.StateReady, st.State)
		assert.Equal(t, 168, st.ActiveBuckets)
		assert.Equal(t, 168, st.ReadyBuckets)
		assert.InDelta(t, 1.0, st.Progress, 1e-9)
	})

	// 5 days of hourly data => 1 sample per bucket, below min_samples => learning.
	t.Run("short history -> learning", func(t *testing.T) {
		store := newFakeStore()
		for ts := now - 5*86400; ts < now; ts += 3600 {
			store.hourly = append(store.hourly, replicaRow("app", "fresh", "fresh-1", ts, 5, 50, 10, 10))
		}
		e := newTestEngine(store, cfg, now)
		require.NoError(t, e.Recompute(context.Background()))

		st, err := store.GetSeriesState(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/fresh", Metric: model.MetricCPU})
		require.NoError(t, err)
		require.NotNil(t, st)
		assert.Equal(t, model.StateLearning, st.State)
		assert.Equal(t, 0, st.ReadyBuckets, "1 sample per bucket is below min_samples=4")
		assert.Less(t, st.Progress, 1.0)
		// Partial coverage (1 of 4 samples per bucket) must still surface progress.
		assert.InDelta(t, 0.25, st.Progress, 1e-9, "1/4 samples per bucket -> 25% coverage")
	})
}

func TestRecomputeOnlyDefaultMetrics(t *testing.T) {
	store := newFakeStore()
	cfg := DefaultConfig() // cpu, memory only
	now := int64(1_700_000_000)
	h := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC).Unix()
	store.hourly = []model.HourlyResourceRow{replicaRow("app", "web", "web-1", h, 10, 100, 100, 50)}

	e := newTestEngine(store, cfg, now)
	require.NoError(t, e.Recompute(context.Background()))

	net, err := store.ListBaselines(context.Background(), model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/web", Metric: model.MetricNetworkIO})
	require.NoError(t, err)
	assert.Empty(t, net, "network_io is not a default metric, so no baseline is written")
}
