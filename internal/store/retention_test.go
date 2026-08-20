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

package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// seedSnapshots inserts count rows ending at the given time, one second apart,
// via a recursive CTE — a Go loop through the serialized writer would dominate
// the test runtime at these volumes.
func seedSnapshots(t *testing.T, db *DB, containerID string, count int, newest time.Time) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(), `
		WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i < `+strconv.Itoa(count-1)+`)
		INSERT INTO resource_snapshots
			(id, container_id, agent_id, cpu_percent, mem_used, mem_limit,
			 net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, timestamp)
		SELECT `+db.Dialect().UUIDExpr()+`, ?, (SELECT agent_id FROM containers WHERE id = ?),
			1.0, 100, 200, 0, 0, 0, 0, ? - i
		FROM n`,
		containerID, containerID, newest.Unix())
	require.NoError(t, err)
}

// seedBuckets fills resource_hourly or resource_daily with count rows.
func seedBuckets(t *testing.T, db *DB, table, containerID string, count int, newest time.Time) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(), `
		WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i < `+strconv.Itoa(count-1)+`)
		INSERT INTO `+table+`
			(id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit,
			 avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
		SELECT `+db.Dialect().UUIDExpr()+`, ?, ? - i*3600, 1.0, 100, 200, 0, 0, 1
		FROM n`,
		containerID, newest.Unix())
	require.NoError(t, err)
}

func countTableRows(t *testing.T, db *DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.ReadDB().QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

// Regression test for issue #46: one call deleted at most batchSize rows, so
// with an hourly cleanup the purge could never keep up past ~3 containers.
func TestDeleteSnapshotsBefore_DeletesBeyondOneBatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-retention", "")
	store := NewResourceStore(db)

	expired := time.Now().Add(-30 * 24 * time.Hour)
	seedSnapshots(t, db, cid, 2500, expired)
	seedSnapshots(t, db, cid, 10, time.Now())
	require.Equal(t, 2510, countTableRows(t, db, "resource_snapshots"))

	deleted, err := store.DeleteSnapshotsBefore(ctx, time.Now().Add(-7*24*time.Hour), 1000)
	require.NoError(t, err)

	assert.Equal(t, int64(2500), deleted, "a single call must drain the whole backlog")
	assert.Equal(t, 10, countTableRows(t, db, "resource_snapshots"), "rows inside the window must survive")
}

func TestDeleteHourlyBefore_DeletesBeyondOneBatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-hourly", "")
	store := NewResourceStore(db)

	// 1500 hourly buckets going back from 100 days ago: all expired.
	seedBuckets(t, db, "resource_hourly", cid, 1500, time.Now().Add(-100*24*time.Hour))

	deleted, err := store.DeleteHourlyBefore(ctx, time.Now().Add(-90*24*time.Hour), 1000)
	require.NoError(t, err)

	assert.Equal(t, int64(1500), deleted)
	assert.Equal(t, 0, countTableRows(t, db, "resource_hourly"))
}

func TestDeleteDailyBefore_DeletesBeyondOneBatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-daily", "")
	store := NewResourceStore(db)

	seedBuckets(t, db, "resource_daily", cid, 1500, time.Now().Add(-400*24*time.Hour))

	deleted, err := store.DeleteDailyBefore(ctx, time.Now().Add(-365*24*time.Hour), 1000)
	require.NoError(t, err)

	assert.Equal(t, int64(1500), deleted)
	assert.Equal(t, 0, countTableRows(t, db, "resource_daily"))
}

// A batch size of zero makes LIMIT 0 return no rows, which the "affected <
// batchSize" exit condition alone reads as "keep going".
func TestDeleteRowsBefore_ZeroBatchSizeDoesNotHang(t *testing.T) {
	db := openTestDB(t)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-zero", "")
	seedSnapshots(t, db, cid, 50, time.Now().Add(-30*24*time.Hour))

	for _, batchSize := range []int{0, -1} {
		t.Run(fmt.Sprintf("batchSize=%d", batchSize), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _, err := deleteRowsBefore(ctx, db.Writer(), batchOpts{batchSize: batchSize},
					"resource_snapshots", "timestamp", time.Now())
				assert.NoError(t, err)
			}()

			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("deleteRowsBefore did not terminate")
			}
		})
	}
	assert.Equal(t, 0, countTableRows(t, db, "resource_snapshots"))
}

func TestDeleteRowsBefore_NoMatchingRows(t *testing.T) {
	db := openTestDB(t)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-none", "")
	seedSnapshots(t, db, cid, 5, time.Now())

	deleted, truncated, err := deleteRowsBefore(context.Background(), db.Writer(),
		batchOpts{batchSize: 1000}, "resource_snapshots", "timestamp", time.Now().Add(-24*time.Hour))

	require.NoError(t, err)
	assert.Zero(t, deleted)
	assert.False(t, truncated)
	assert.Equal(t, 5, countTableRows(t, db, "resource_snapshots"))
}

// Shutdown must not surface as an error: the writer refuses new work once the
// context is cancelled, and the rows already deleted are committed.
func TestDeleteRowsBefore_HonoursContextCancellation(t *testing.T) {
	db := openTestDB(t)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-cancel", "")
	seedSnapshots(t, db, cid, 500, time.Now().Add(-30*24*time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deleted, truncated, err := deleteRowsBefore(ctx, db.Writer(), batchOpts{batchSize: 100},
		"resource_snapshots", "timestamp", time.Now())

	require.NoError(t, err)
	assert.Zero(t, deleted)
	assert.True(t, truncated)
	assert.Equal(t, 500, countTableRows(t, db, "resource_snapshots"))
}

func TestRunBatchedDelete_BudgetTruncates(t *testing.T) {
	calls := 0
	total, truncated, err := runBatchedDelete(context.Background(),
		batchOpts{batchSize: 10, budget: time.Nanosecond},
		func(context.Context, int) (int64, error) {
			calls++
			return 10, nil // always a full batch: only the budget can stop this
		})

	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Equal(t, 1, calls, "the budget must be checked after the first batch")
	assert.Equal(t, int64(10), total)
}

func TestRunResourceCleanup_PurgesAllThreeTables(t *testing.T) {
	db := openTestDB(t)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-all", "")
	store := NewResourceStore(db)

	seedSnapshots(t, db, cid, 1200, time.Now().Add(-30*24*time.Hour))
	seedSnapshots(t, db, cid, 5, time.Now())
	seedBuckets(t, db, "resource_hourly", cid, 1200, time.Now().Add(-100*24*time.Hour))
	seedBuckets(t, db, "resource_daily", cid, 1200, time.Now().Add(-400*24*time.Hour))

	cfg := RetentionConfig{}.withDefaults(testLogger())
	var pass retentionPass
	runResourceCleanup(context.Background(), store, testLogger(), cfg, &pass)

	assert.False(t, pass.truncated)
	assert.Equal(t, int64(3600), pass.deleted)
	assert.Equal(t, 5, countTableRows(t, db, "resource_snapshots"))
	assert.Equal(t, 0, countTableRows(t, db, "resource_hourly"))
	assert.Equal(t, 0, countTableRows(t, db, "resource_daily"))
}

// The cleanup used to wait a full interval before its first run, so an instance
// restarting more often than that never purged anything.
func TestStartRetentionCleanup_RunsImmediately(t *testing.T) {
	db := openTestDB(t)
	cid := seedHostContainer(t, NewContainerStore(db), "ext-immediate", "")
	seedSnapshots(t, db, cid, 100, time.Now().Add(-30*24*time.Hour))

	ctx := t.Context()

	// An hour-long interval guarantees the timer never fires during the test:
	// anything deleted came from the immediate first pass.
	StartRetentionCleanupWithOpts(ctx, NewContainerStore(db), db, testLogger(), RetentionOpts{
		ResourceStore: NewResourceStore(db),
		Config:        RetentionConfig{Interval: time.Hour},
	})

	require.Eventually(t, func() bool {
		return countTableRows(t, db, "resource_snapshots") == 0
	}, 5*time.Second, 20*time.Millisecond, "the first pass must run without waiting for the interval")
}

// A pass that stopped on its budget must come back quickly; waiting a full hour
// is what let the backlog grow in the first place.
func TestRetentionConfig_NextDelay(t *testing.T) {
	cfg := RetentionConfig{}.withDefaults(testLogger())

	assert.Equal(t, retentionInterval, cfg.nextDelay(false))
	assert.Equal(t, retentionCatchUpInterval, cfg.nextDelay(true))
	assert.Less(t, cfg.nextDelay(true), cfg.nextDelay(false))
}

func TestRetentionConfig_Defaults(t *testing.T) {
	logger := testLogger()

	t.Run("zero value keeps historical behaviour", func(t *testing.T) {
		cfg := RetentionConfig{}.withDefaults(logger)
		assert.Equal(t, retentionInterval, cfg.Interval)
		assert.Equal(t, retentionBatchSize, cfg.BatchSize)
		assert.Equal(t, resourceSnapshotRetention, cfg.Snapshots)
		assert.Equal(t, resourceHourlyRetention, cfg.Hourly)
		assert.Equal(t, resourceDailyRetention, cfg.Daily)
		assert.Equal(t, defaultRetention, cfg.Transitions)
		assert.Equal(t, certCheckResultRetention, cfg.CertCheckResults)
	})

	t.Run("out of range values are clamped", func(t *testing.T) {
		cfg := RetentionConfig{
			Interval:       time.Second,
			BatchSize:      1,
			Snapshots:      time.Minute,
			BudgetPerTable: time.Hour,
		}.withDefaults(logger)

		assert.Equal(t, time.Minute, cfg.Interval)
		assert.Equal(t, 100, cfg.BatchSize)
		assert.Equal(t, 24*time.Hour, cfg.Snapshots, "raw window feeds the 7-day graphs")
		assert.Equal(t, 30*time.Minute, cfg.BudgetPerTable)
	})

	t.Run("batch size above maximum is clamped", func(t *testing.T) {
		cfg := RetentionConfig{BatchSize: 500000}.withDefaults(logger)
		assert.Equal(t, 100000, cfg.BatchSize)
	})
}
