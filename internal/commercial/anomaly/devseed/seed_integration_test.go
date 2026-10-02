// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package devseed_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
	"github.com/kolapsis/maintenant/internal/commercial/anomaly/devseed"
	"github.com/kolapsis/maintenant/internal/store"
)

const seedSpan = 28 * 24 * time.Hour

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// openDB returns a migrated database and its path, next to which the seeder keeps its manifest.
func openDB(t *testing.T) (*store.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devseed.db")
	db, err := store.Open(path, testLogger())
	require.NoError(t, err)
	require.NoError(t, store.Migrate(context.Background(), db, testLogger()))
	ctx, cancel := context.WithCancel(context.Background())
	db.StartWriter(ctx)
	t.Cleanup(func() { cancel(); _ = db.Close() })
	return db, path
}

func insertContainer(t *testing.T, db *store.DB, id string, archived, ignored int) {
	t.Helper()
	now := time.Now().Unix()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO containers (id, agent_id, external_id, name, image, state, runtime_type,
			archived, is_ignored, first_seen_at, last_state_change_at)
		VALUES (?, '00000000-0000-0000-0000-000000000000', ?, ?, 'img:latest', 'running', 'docker', ?, ?, ?, ?)`,
		id, "ext-"+id, id, archived, ignored, now, now)
	require.NoError(t, err)
}

func insertHourly(t *testing.T, db *store.DB, containerID string, bucket int64, cpu, mem float64) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO resource_hourly (id, container_id, bucket, avg_cpu_percent, avg_mem_used,
			avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		VALUES (?, ?, ?, ?, ?, 1000000000, 1000, 2000, 300)`,
		fmt.Sprintf("rh-%s-%d", containerID, bucket), containerID, bucket, cpu, mem)
	require.NoError(t, err)
}

// seedPartialHistory writes days of hourly rollups with one hour in four missing, like an install that does not run continuously.
func seedPartialHistory(t *testing.T, db *store.DB, containerID string, days int, cpuAt func(int) float64) map[int64]float64 {
	t.Helper()
	written := map[int64]float64{}
	currentHour := time.Now().Truncate(time.Hour).Unix()
	start := currentHour - int64(days)*86400
	for ts, i := start, 0; ts < currentHour; ts, i = ts+3600, i+1 {
		if i%4 == 3 { // one hour in four is missing
			continue
		}
		cpu := cpuAt(i)
		insertHourly(t, db, containerID, ts, cpu, 5e7)
		written[ts] = cpu
	}
	return written
}

func countHourly(t *testing.T, db *store.DB, containerID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.ReadDB().QueryRow(
		`SELECT COUNT(*) FROM resource_hourly WHERE container_id = ?`, containerID).Scan(&n))
	return n
}

// A container with gappy partial history reaches ready right after seeding.
func TestSeedThenRecomputeReachesReady(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-api", 0, 0)
	seedPartialHistory(t, db, "ctr-api", 13, func(i int) float64 { return 20 + 5*math.Sin(float64(i)/4) })

	rep, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Seeded)
	assert.Positive(t, rep.RowsWritten)

	// The window is covered hour by hour, up to and including the hour in progress.
	assert.Equal(t, int(seedSpan/time.Hour)+1, countHourly(t, db, "ctr-api"))

	as := store.NewAnomalyStore(db)
	cfg := anomaly.DefaultConfig()
	sm := anomaly.NewSeriesStateManager(as, cfg, testLogger())
	engine := anomaly.NewBaselineEngine(as, sm, cfg, testLogger())
	require.NoError(t, engine.Recompute(context.Background()))

	states, err := as.ListSeriesStates(context.Background(), model.ScopeTypeContainer)
	require.NoError(t, err)
	require.NotEmpty(t, states)
	for _, st := range states {
		assert.Equal(t, model.StateReady, st.State, "metric %s", st.Metric)
		assert.InDelta(t, 1.0, st.Progress, 1e-9, "metric %s", st.Metric)
		assert.Equal(t, st.ActiveBuckets, st.ReadyBuckets, "metric %s", st.Metric)
		assert.Equal(t, anomaly.BucketsPerWeek, st.ActiveBuckets, "metric %s", st.Metric)

		baselines, err := as.ListBaselines(context.Background(), st.SeriesKey)
		require.NoError(t, err)
		assert.Len(t, baselines, anomaly.BucketsPerWeek)
		for _, b := range baselines {
			assert.GreaterOrEqual(t, b.SampleCount, cfg.MinSamples, "bucket %d", b.Bucket)
		}
	}
}

func TestSeedNeverOverwritesRealRows(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-api", 0, 0)
	real := seedPartialHistory(t, db, "ctr-api", 13, func(i int) float64 { return 30 + float64(i%7) })

	_, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)

	rows, err := db.ReadDB().Query(
		`SELECT bucket, avg_cpu_percent FROM resource_hourly WHERE container_id = 'ctr-api'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	checked := 0
	for rows.Next() {
		var bucket int64
		var cpu float64
		require.NoError(t, rows.Scan(&bucket, &cpu))
		if want, ok := real[bucket]; ok {
			assert.InDelta(t, want, cpu, 1e-9, "real row at %d was rewritten", bucket)
			checked++
		}
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, len(real), checked, "every real row must still be there")
}

func TestSeedIsIdempotent(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-api", 0, 0)
	seedPartialHistory(t, db, "ctr-api", 13, func(int) float64 { return 12 })

	first, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)
	require.Positive(t, first.RowsWritten)
	after := countHourly(t, db, "ctr-api")

	second, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)
	assert.Zero(t, second.RowsWritten, "a second run must find nothing to do")
	assert.Equal(t, after, countHourly(t, db, "ctr-api"))
}

func TestSeedSkipsArchivedIgnoredAndThinContainers(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-archived", 1, 0)
	insertContainer(t, db, "ctr-ignored", 0, 1)
	insertContainer(t, db, "ctr-thin", 0, 0)
	insertContainer(t, db, "ctr-empty", 0, 0)

	seedPartialHistory(t, db, "ctr-archived", 13, func(int) float64 { return 10 })
	seedPartialHistory(t, db, "ctr-ignored", 13, func(int) float64 { return 10 })
	// One day of history is below the two-day floor.
	seedPartialHistory(t, db, "ctr-thin", 1, func(int) float64 { return 10 })

	before := map[string]int{}
	for _, id := range []string{"ctr-archived", "ctr-ignored", "ctr-thin", "ctr-empty"} {
		before[id] = countHourly(t, db, id)
	}

	rep, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)
	assert.Zero(t, rep.Seeded)
	assert.Equal(t, 2, rep.Skipped, "only the two non-archived, non-ignored containers are candidates")

	for id, n := range before {
		assert.Equal(t, n, countHourly(t, db, id), "%s must be untouched", id)
	}
}

// The band must come from the learned MAD, not from EffectiveMAD's floor.
func TestSeedProducesLearnedSpread(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-busy", 0, 0)
	seedPartialHistory(t, db, "ctr-busy", 13, func(i int) float64 { return 40 + 12*math.Sin(float64(i)/3) })

	_, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)

	as := store.NewAnomalyStore(db)
	cfg := anomaly.DefaultConfig()
	sm := anomaly.NewSeriesStateManager(as, cfg, testLogger())
	require.NoError(t, anomaly.NewBaselineEngine(as, sm, cfg, testLogger()).Recompute(context.Background()))

	key := model.SeriesKey{ScopeType: model.ScopeTypeContainer, Metric: model.MetricCPU}
	states, err := as.ListSeriesStates(context.Background(), model.ScopeTypeContainer)
	require.NoError(t, err)
	for _, st := range states {
		if st.Metric == model.MetricCPU {
			key = st.SeriesKey
		}
	}

	baselines, err := as.ListBaselines(context.Background(), key)
	require.NoError(t, err)
	require.NotEmpty(t, baselines)

	overFloor := 0
	for _, b := range baselines {
		if b.MAD > anomaly.RelativeMADFloor*b.Median && b.MAD > anomaly.AbsoluteMADFloor(model.MetricCPU) {
			overFloor++
		}
	}
	assert.Positive(t, overFloor, "a varying container must learn a MAD above the floors")
}

func TestResetClearsLearnedState(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-api", 0, 0)
	seededRealBuckets := seedPartialHistory(t, db, "ctr-api", 13, func(int) float64 { return 15 })

	_, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)

	as := store.NewAnomalyStore(db)
	cfg := anomaly.DefaultConfig()
	sm := anomaly.NewSeriesStateManager(as, cfg, testLogger())
	require.NoError(t, anomaly.NewBaselineEngine(as, sm, cfg, testLogger()).Recompute(context.Background()))

	states, err := as.ListSeriesStates(context.Background(), model.ScopeTypeContainer)
	require.NoError(t, err)
	require.NotEmpty(t, states)

	// Reset alone withdraws the synthetic hours and drops learning, keeping every measured rollup.
	measured := len(seededRealBuckets)
	_, err = devseed.Seed(context.Background(), db, dbFile, 0, true, testLogger())
	require.NoError(t, err)

	states, err = as.ListSeriesStates(context.Background(), model.ScopeTypeContainer)
	require.NoError(t, err)
	assert.Empty(t, states)
	assert.Equal(t, measured, countHourly(t, db, "ctr-api"), "measured rollups must survive a reset")
}

// Without the manifest nothing is withdrawn.
func TestWithdrawWithoutManifestKeepsEverything(t *testing.T) {
	db, dbFile := openDB(t)
	insertContainer(t, db, "ctr-api", 0, 0)
	seedPartialHistory(t, db, "ctr-api", 13, func(int) float64 { return 15 })

	_, err := devseed.Seed(context.Background(), db, dbFile, seedSpan, false, testLogger())
	require.NoError(t, err)
	total := countHourly(t, db, "ctr-api")

	require.NoError(t, os.Remove(dbFile+".devseed.json"))

	rep, err := devseed.Seed(context.Background(), db, dbFile, 0, true, testLogger())
	require.NoError(t, err)
	assert.Zero(t, rep.RowsWithdrawn)
	assert.Equal(t, total, countHourly(t, db, "ctr-api"))
}
