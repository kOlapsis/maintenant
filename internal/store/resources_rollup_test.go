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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/resource"
)

// The 7-day chart is served from resource_hourly, so the rollup has to carry
// block I/O too — without it that range would lose its Block I/O series.
func TestAggregateHourlyRollup_CarriesBlockIO(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-rollup", "")
	store := NewResourceStore(db)

	bucket := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Hour)
	for i, sample := range []struct{ read, write int64 }{{100, 1000}, {300, 3000}} {
		_, err := store.InsertSnapshot(ctx, &resource.ResourceSnapshot{
			ContainerID:     cid,
			CPUPercent:      10,
			MemUsed:         200,
			MemLimit:        1000,
			NetRxBytes:      5,
			NetTxBytes:      7,
			BlockReadBytes:  sample.read,
			BlockWriteBytes: sample.write,
			Timestamp:       bucket.Add(time.Duration(i) * time.Minute),
		})
		require.NoError(t, err)
	}

	require.NoError(t, store.AggregateHourlyRollup(ctx, bucket, bucket.Add(time.Hour)))

	rows, err := store.ListHourlyInRange(ctx, cid, bucket.Add(-time.Hour), bucket.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.EqualValues(t, 200, rows[0].BlockReadBytes, "average of 100 and 300")
	assert.EqualValues(t, 2000, rows[0].BlockWriteBytes, "average of 1000 and 3000")
	assert.EqualValues(t, 10, rows[0].CPUPercent)
	assert.Equal(t, bucket.Unix(), rows[0].Timestamp.Unix(), "the bucket is the point timestamp")
}

// A re-run over the same bucket must refresh the aggregate rather than insert a
// second row: the rollup replays its whole window every few minutes.
func TestAggregateHourlyRollup_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-rollup-idem", "")
	store := NewResourceStore(db)

	bucket := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Hour)
	_, err := store.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 1, MemUsed: 1, MemLimit: 2,
		BlockReadBytes: 50, Timestamp: bucket.Add(time.Minute),
	})
	require.NoError(t, err)

	require.NoError(t, store.AggregateHourlyRollup(ctx, bucket, bucket.Add(time.Hour)))
	require.NoError(t, store.AggregateHourlyRollup(ctx, bucket, bucket.Add(time.Hour)))

	rows, err := store.ListHourlyInRange(ctx, cid, bucket, bucket.Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.EqualValues(t, 50, rows[0].BlockReadBytes)
}

// An empty window must leave whatever was already aggregated alone, otherwise
// purged raw data would wipe the rollup that replaced it.
func TestAggregateHourlyRollup_EmptyWindowKeepsExistingBucket(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cid := seedHostContainer(t, NewContainerStore(db), "ext-rollup-empty", "")
	store := NewResourceStore(db)

	bucket := time.Now().Add(-5 * time.Hour).UTC().Truncate(time.Hour)
	_, err := store.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 42, MemUsed: 1, MemLimit: 2,
		Timestamp: bucket.Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, store.AggregateHourlyRollup(ctx, bucket, bucket.Add(time.Hour)))

	// Raw data for that hour is gone, as retention would leave it.
	_, err = db.Writer().Exec(ctx, "DELETE FROM resource_snapshots WHERE container_id = ?", cid)
	require.NoError(t, err)
	require.NoError(t, store.AggregateHourlyRollup(ctx, bucket, bucket.Add(time.Hour)))

	rows, err := store.ListHourlyInRange(ctx, cid, bucket, bucket.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 42, rows[0].CPUPercent, "the aggregate must outlive the raw rows")
}
