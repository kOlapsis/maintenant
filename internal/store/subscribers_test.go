// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

func pendingSubscription(email, confirmToken, unsubToken string) *status.StatusSubscriber {
	expires := time.Now().Add(24 * time.Hour)
	return &status.StatusSubscriber{
		Email:          email,
		ConfirmToken:   &confirmToken,
		ConfirmExpires: &expires,
		UnsubToken:     unsubToken,
	}
}

func TestUpsertPendingSubscriber_NewPendingConfirmed(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriberStore(db)
	ctx := context.Background()

	issued, err := s.UpsertPendingSubscriber(ctx, pendingSubscription("visitor@example.com", "confirm-1", "unsub-1"))
	require.NoError(t, err)
	assert.True(t, issued, "a new address gets a confirmation token")

	issued, err = s.UpsertPendingSubscriber(ctx, pendingSubscription("visitor@example.com", "confirm-2", "unsub-2"))
	require.NoError(t, err)
	assert.True(t, issued, "a pending address gets a fresh confirmation token")

	stale, err := s.GetSubscriberByToken(ctx, "confirm-1")
	require.NoError(t, err)
	assert.Nil(t, stale, "the previous confirmation link stops working")
	pending, err := s.GetSubscriberByToken(ctx, "confirm-2")
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, "unsub-1", pending.UnsubToken, "the unsubscribe link of the address does not change")
	assert.False(t, pending.Confirmed)

	require.NoError(t, s.ConfirmSubscriber(ctx, pending.ID))

	issued, err = s.UpsertPendingSubscriber(ctx, pendingSubscription("visitor@example.com", "confirm-3", "unsub-3"))
	require.NoError(t, err)
	assert.False(t, issued, "a confirmed address gets no confirmation token")

	again, err := s.GetSubscriberByToken(ctx, "confirm-3")
	require.NoError(t, err)
	assert.Nil(t, again)
	confirmed, err := s.GetSubscriberByUnsubToken(ctx, "unsub-1")
	require.NoError(t, err)
	require.NotNil(t, confirmed)
	assert.True(t, confirmed.Confirmed, "subscribing again never unconfirms an address")
	assert.Nil(t, confirmed.ConfirmToken)

	stats, err := s.GetSubscriberStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total)
	assert.Equal(t, 1, stats.Confirmed)
}

func TestUpsertPendingSubscriber_RefreshKeepsAPendingAddressAlive(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriberStore(db)
	ctx := context.Background()

	_, err := s.UpsertPendingSubscriber(ctx, pendingSubscription("visitor@example.com", "confirm-1", "unsub-1"))
	require.NoError(t, err)
	_, err = db.Writer().Exec(ctx, `UPDATE status_subscribers SET created_at = ? WHERE email = ?`,
		time.Now().Add(-25*time.Hour).Unix(), "visitor@example.com")
	require.NoError(t, err)

	_, err = s.UpsertPendingSubscriber(ctx, pendingSubscription("visitor@example.com", "confirm-2", "unsub-2"))
	require.NoError(t, err)

	deleted, err := s.CleanExpiredUnconfirmed(ctx)
	require.NoError(t, err)
	assert.Zero(t, deleted, "a fresh confirmation link must not be swept by the 24 h cleanup")
}
