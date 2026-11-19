// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestAlertStore_PurgeSpansSeveralBatchesAndNullsSurvivorReferences(t *testing.T) {
	db := openTestDB(t)
	s := NewAlertStore(db)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-24 * time.Hour)
	cutoff := now.Add(-90 * 24 * time.Hour)

	const purgeable = 25
	var purgedIDs []string
	for i := 0; i < purgeable; i++ {
		purgedIDs = append(purgedIDs, seedAlert(t, s, "old-"+strconv.Itoa(i), alert.StatusResolved, old, &old))
	}
	survivor := seedAlert(t, s, "survivor", alert.StatusResolved, recent, &recent)
	doomed := seedAlert(t, s, "doomed", alert.StatusResolved, old, &old)
	require.NoError(t, s.UpdateAlertStatus(ctx, survivor, alert.StatusResolved, &recent, &doomed))
	got, err := s.GetAlert(ctx, survivor)
	require.NoError(t, err)
	require.NotNil(t, got.ResolvedByID)

	deleted, truncated, err := s.deleteInactiveAlertsOlderThan(ctx, cutoff, batchOpts{batchSize: 7})
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.EqualValues(t, purgeable+1, deleted)

	for _, id := range append(purgedIDs, doomed) {
		a, err := s.GetAlert(ctx, id)
		require.NoError(t, err)
		assert.Nil(t, a, "alert %s should be purged", id)
	}
	got, err = s.GetAlert(ctx, survivor)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ResolvedByID, "reference to a purged alert must be nulled")
}

func TestAlertStore_ResolvedByIDIsIndexed(t *testing.T) {
	db := openTestDB(t)
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_alerts_resolved_by_id'`
	if db.Dialect() == DialectPostgres {
		query = `SELECT COUNT(*) FROM pg_indexes WHERE indexname='idx_alerts_resolved_by_id'`
	}
	var n int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), query).Scan(&n))
	assert.Equal(t, 1, n)
}
