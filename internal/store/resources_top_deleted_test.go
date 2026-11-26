// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/resource"
)

func TestGetTopConsumersByPeriod_LeavesOutDeletedContainers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	store := NewResourceStore(db)

	live := seedHostContainer(t, cstore, "ext-top-live", "")
	gone := seedHostContainer(t, cstore, "ext-top-gone", "")

	now := time.Now().UTC()
	day := startOfUTCDay(now).AddDate(0, 0, -3)
	hour := now.Truncate(time.Hour).Add(-3 * time.Hour)
	for cid, cpu := range map[string]float64{live: 10, gone: 90} {
		row := &resource.RollupRow{ContainerID: cid, AvgCPUPercent: cpu, AvgMemUsed: 1, AvgMemLimit: 2, SampleCount: 24}
		row.Bucket = day
		require.NoError(t, store.InsertDailyRollup(ctx, row))
		row.Bucket = hour
		require.NoError(t, store.InsertHourlyRollup(ctx, row))
	}

	require.NoError(t, cstore.DeleteContainerByID(ctx, gone))

	for _, period := range []string{"24h", "7d", "30d", "90d"} {
		rows, err := store.GetTopConsumersByPeriod(ctx, "cpu", period, 10, nil, now)
		require.NoError(t, err, period)
		require.Len(t, rows, 1, period)
		assert.Equal(t, live, rows[0].ContainerID, period)
	}
}
