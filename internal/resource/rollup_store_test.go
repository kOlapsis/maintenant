// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package resource_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestRollupHourly_KeepsTheFullHourOnceRetentionCutsIntoIt(t *testing.T) {
	db := storetest.Open(t, nil)
	ctx := context.Background()
	const rawWindow = 24 * time.Hour

	seen := time.Now()
	cid, err := store.NewContainerStore(db).InsertContainer(ctx, &container.Container{
		ExternalID: "ext-rollup-edge", Name: "rollup-edge", Image: "img:v1",
		State: container.StateRunning, AlertSeverity: container.SeverityWarning,
		RestartThreshold: 3, RuntimeType: "docker", FirstSeenAt: seen, LastStateChangeAt: seen,
	})
	require.NoError(t, err)
	rs := store.NewResourceStore(db)

	hour := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	for minute := 0; minute < 60; minute++ {
		_, err := rs.InsertSnapshot(ctx, &resource.ResourceSnapshot{
			ContainerID: cid, CPUPercent: float64(minute), MemUsed: 1, MemLimit: 2,
			Timestamp: hour.Add(time.Duration(minute) * time.Minute),
		})
		require.NoError(t, err)
	}

	bucket := func() (avg float64, samples int) {
		t.Helper()
		require.NoError(t, db.Reader().QueryRowContext(ctx,
			`SELECT avg_cpu_percent, sample_count FROM resource_hourly WHERE container_id = ? AND bucket = ?`,
			cid, hour.Unix()).Scan(&avg, &samples))
		return avg, samples
	}

	resource.RollupHourlyAt(ctx, rs, rawWindow, hour.Add(time.Hour+5*time.Minute))
	avg, samples := bucket()
	require.InDelta(t, 29.5, avg, 0.001)
	require.Equal(t, 60, samples)

	cutoff := hour.Add(40 * time.Minute)
	_, err = rs.DeleteSnapshotsBefore(ctx, cutoff, 1000)
	require.NoError(t, err)

	resource.RollupHourlyAt(ctx, rs, rawWindow, cutoff.Add(rawWindow))
	avg, samples = bucket()
	assert.InDelta(t, 29.5, avg, 0.001)
	assert.Equal(t, 60, samples)
}
