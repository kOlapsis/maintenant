// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func openDB(t *testing.T) *store.DB {
	t.Helper()
	return storetest.Open(t, testLogger())
}

func insertContainer(t *testing.T, db *store.DB, id, group, unit string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO containers (id, agent_id, external_id, name, image, state, orchestration_group, orchestration_unit, runtime_type, first_seen_at, last_state_change_at)
		VALUES (?, '00000000-0000-0000-0000-000000000000', ?, ?, 'img:latest', 'running', ?, ?, 'docker', ?, ?)`,
		id, "ext-"+id, unit+"-1", group, unit, now, now)
	require.NoError(t, err)
}

func insertHourly(t *testing.T, db *store.DB, containerID string, bucket int64, cpu, mem float64, count int) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO resource_hourly (id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, 0, 0, 0, ?)`,
		"rh-"+containerID+"-"+itoa64(bucket), containerID, bucket, cpu, mem, count)
	require.NoError(t, err)
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func TestBaselineIntegrationReachesReady(t *testing.T) {
	db := openDB(t)
	as := store.NewAnomalyStore(db)
	insertContainer(t, db, "ctr-api", "app", "api")

	// One hour past now: the engine's clock moves on during the inserts, and the oldest bucket would fall a sample short.
	now := time.Now().Unix()
	for ts := now - 31*86400; ts < now+3600; ts += 3600 {
		insertHourly(t, db, "ctr-api", ts, 5.0, 50_000_000, 6)
	}

	cfg := anomaly.DefaultConfig()
	sm := anomaly.NewSeriesStateManager(as, cfg, testLogger())
	engine := anomaly.NewBaselineEngine(as, sm, cfg, testLogger())
	require.NoError(t, engine.Recompute(context.Background()))

	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/api", Metric: model.MetricCPU}

	baselines, err := as.ListBaselines(context.Background(), key)
	require.NoError(t, err)
	assert.Equal(t, anomaly.BucketsPerWeek, len(baselines), "all 168 weekly buckets learned")
	for _, b := range baselines {
		assert.InDelta(t, 5.0, b.Median, 1e-6)
		assert.GreaterOrEqual(t, b.SampleCount, cfg.MinSamples)
	}

	st, err := as.GetSeriesState(context.Background(), key)
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, model.StateReady, st.State)
	assert.InDelta(t, 1.0, st.Progress, 1e-9)
}

func TestBaselineIntegrationShortHistoryStaysLearning(t *testing.T) {
	db := openDB(t)
	as := store.NewAnomalyStore(db)
	insertContainer(t, db, "ctr-fresh", "app", "fresh")

	now := time.Now().Unix()
	for ts := now - 3*86400; ts < now; ts += 3600 {
		insertHourly(t, db, "ctr-fresh", ts, 5.0, 50_000_000, 6)
	}

	cfg := anomaly.DefaultConfig()
	sm := anomaly.NewSeriesStateManager(as, cfg, testLogger())
	engine := anomaly.NewBaselineEngine(as, sm, cfg, testLogger())
	require.NoError(t, engine.Recompute(context.Background()))

	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, ScopeID: "compose/app/fresh", Metric: model.MetricCPU}
	st, err := as.GetSeriesState(context.Background(), key)
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, model.StateLearning, st.State, "3 days of history is not enough to be ready")
}
