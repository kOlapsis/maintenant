// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
)

// waitingDelivery reserves a delivery whose first attempt failed and whose retry is due at due.
func (g guardedRun) waitingDelivery(t *testing.T, level int, due time.Time) *escalation.Delivery {
	t.Helper()
	ctx := context.Background()
	d := g.levelDelivery()
	d.LevelIndex = level
	d.Attempts, d.NextAttemptAt = 1, time.Now().Add(time.Hour)
	_, err := g.runs.InsertDelivery(ctx, d)
	require.NoError(t, err)
	claim := d.NextAttemptAt
	d.Error, d.NextAttemptAt = "unreachable", due
	settled, err := g.runs.SettleDelivery(ctx, d, claim)
	require.NoError(t, err)
	require.True(t, settled)
	return d
}

func (g guardedRun) due(t *testing.T, now time.Time) []*escalation.Delivery {
	t.Helper()
	due, err := g.runs.SelectDueDeliveries(context.Background(), now)
	require.NoError(t, err)
	return due
}

func TestEscalationDeliveryQueue_ARetryIsDueAtItsNextAttempt(t *testing.T) {
	g := newGuardedRun(t)
	next := time.Now().Add(10 * time.Second).Truncate(time.Second)
	d := g.waitingDelivery(t, 0, next)

	assert.Empty(t, g.due(t, next.Add(-time.Second)), "a retry is not due before its time")
	due := g.due(t, next)
	require.Len(t, due, 1)
	assert.Equal(t, d.ID, due[0].ID)
	assert.Equal(t, 1, due[0].Attempts)
	assert.Equal(t, "unreachable", due[0].Error)
	assert.Equal(t, due[0].AttemptStartedAt, due[0].CreatedAt, "a delivery is created by its first attempt")
}

func TestEscalationDeliveryQueue_AnAttemptIsTakenOnce(t *testing.T) {
	g := newGuardedRun(t)
	ctx := context.Background()
	now := time.Now()
	g.waitingDelivery(t, 0, now)
	seen := g.due(t, now)[0]

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			d := *seen
			ok, err := g.runs.ClaimDelivery(ctx, &d, now, now.Add(time.Minute))
			if err == nil && ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load(), "one worker takes the attempt")
	assert.Empty(t, g.due(t, now.Add(30*time.Second)), "a taken attempt is not due while it is held")

	stale := *seen
	stale.Status = escalation.DeliveryStatusSent
	settled, err := g.runs.SettleDelivery(ctx, &stale, seen.NextAttemptAt)
	require.NoError(t, err)
	assert.False(t, settled, "an outcome is only recorded under the claim that holds")
}

func TestEscalationDeliveryQueue_ALevelIsNotTakenOnceItsRunStops(t *testing.T) {
	g := newGuardedRun(t)
	ctx := context.Background()
	now := time.Now()
	g.waitingDelivery(t, 0, now)
	g.waitingDelivery(t, -1, now)
	require.NoError(t, g.runs.TerminateRun(ctx, g.run.ID, escalation.RunStatusStoppedByAck, now))

	byLevel := map[int]*escalation.Delivery{}
	for _, d := range g.due(t, now) {
		byLevel[d.LevelIndex] = d
	}
	ok, err := g.runs.ClaimDelivery(ctx, byLevel[0], now, now.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, ok, "a level does not go out once its run has stopped")
	ok, err = g.runs.ClaimDelivery(ctx, byLevel[-1], now, now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, ok, "a notice sent after the run stopped can still be retried")
}
