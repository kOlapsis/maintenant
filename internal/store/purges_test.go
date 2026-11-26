// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/update"
)

const day = 24 * time.Hour

func TestCleanupExpired_PurgesOldRiskScoreHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)
	now := time.Now()
	for _, age := range []time.Duration{100 * day, 91 * day, 89 * day, 0} {
		_, err := us.InsertRiskScoreRecord(ctx, &update.RiskScoreRecord{
			ContainerID: "ctr-1", Score: 50, FactorsJSON: "{}", RecordedAt: now.Add(-age),
		})
		require.NoError(t, err)
	}

	deleted, err := us.CleanupExpired(ctx, now.Add(-30*day))
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	kept, err := us.ListRiskScoreHistory(ctx, "", "ctr-1", now.Add(-365*day), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, kept, 2)
}

func TestRetentionPass_PurgesSilenceRulesEndedLongAgo(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewSilenceStore(db)
	now := time.Now()
	rule := func(startedAgo, duration time.Duration) string {
		t.Helper()
		id, err := s.InsertSilenceRule(ctx, &alert.SilenceRule{
			Reason: "maintenance", StartsAt: now.Add(-startedAgo), DurationSeconds: int(duration.Seconds()),
		})
		require.NoError(t, err)
		return id
	}
	rule(100*day, time.Hour)
	cancelledLongAgo := rule(100*day, 365*day)
	_, err := db.db.Exec(db.dialect.Rebind(`UPDATE silence_rules SET cancelled_at = ? WHERE id = ?`),
		now.Add(-95*day).Unix(), cancelledLongAgo)
	require.NoError(t, err)
	endedRecently := rule(10*day, time.Hour)
	cancelledRecently := rule(100*day, 365*day)
	require.NoError(t, s.CancelSilenceRule(ctx, cancelledRecently))
	active := rule(100*day, 365*day)

	logger := testLogger()
	runRetentionPass(ctx, NewContainerStore(db), db, logger, RetentionOpts{}, RetentionConfig{}.withDefaults(logger))

	rules, err := s.ListSilenceRules(ctx, false)
	require.NoError(t, err)
	var kept []string
	for _, r := range rules {
		kept = append(kept, r.ID)
	}
	assert.ElementsMatch(t, []string{endedRecently, cancelledRecently, active}, kept)
}
