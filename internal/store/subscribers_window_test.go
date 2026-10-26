// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
)

// Acceptance tests of 047-audit-small-fixes, C13: a pending address is not mailed again within 15 minutes.

func backdateSubscriber(t *testing.T, db *DB, email string, age time.Duration) {
	t.Helper()
	_, err := db.Writer().Exec(context.Background(),
		`UPDATE status_subscribers SET created_at = ? WHERE email = ?`, time.Now().Add(-age).Unix(), email)
	require.NoError(t, err)
}

// FR-004, US2 scenario 1: no new confirmation inside the 15-minute window, a new one past it.
func TestUpsertPendingSubscriber_HoldsAPendingAddressForFifteenMinutes(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriberStore(db)
	ctx := context.Background()
	const email = "visitor@example.com"

	issued, err := s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-1", "unsub-1"))
	require.NoError(t, err)
	require.True(t, issued)

	issued, err = s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-2", "unsub-2"))
	require.NoError(t, err)
	assert.False(t, issued, "a request right after the previous one issues nothing")
	first, err := s.GetSubscriberByToken(ctx, "confirm-1")
	require.NoError(t, err)
	assert.NotNil(t, first, "the link already sent keeps working meanwhile")

	backdateSubscriber(t, db, email, 14*time.Minute)
	issued, err = s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-3", "unsub-3"))
	require.NoError(t, err)
	assert.False(t, issued, "14 minutes is still inside the window")

	backdateSubscriber(t, db, email, 16*time.Minute)
	issued, err = s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-4", "unsub-4"))
	require.NoError(t, err)
	assert.True(t, issued, "past 15 minutes a new confirmation is issued")
	fresh, err := s.GetSubscriberByToken(ctx, "confirm-4")
	require.NoError(t, err)
	require.NotNil(t, fresh)
	assert.Equal(t, "unsub-1", fresh.UnsubToken)
	stale, err := s.GetSubscriberByToken(ctx, "confirm-1")
	require.NoError(t, err)
	assert.Nil(t, stale, "the new link replaces the old one")
}

// Edge case "adresse déjà confirmée": still no confirmation, whatever the age of the row.
func TestUpsertPendingSubscriber_AConfirmedAddressNeverGetsAConfirmation(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriberStore(db)
	ctx := context.Background()
	const email = "member@example.com"

	_, err := s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-1", "unsub-1"))
	require.NoError(t, err)
	sub, err := s.GetSubscriberByToken(ctx, "confirm-1")
	require.NoError(t, err)
	require.NoError(t, s.ConfirmSubscriber(ctx, sub.ID))
	backdateSubscriber(t, db, email, time.Hour)

	issued, err := s.UpsertPendingSubscriber(ctx, pendingSubscription(email, "confirm-2", "unsub-2"))
	require.NoError(t, err)
	assert.False(t, issued)
}

type mailbox struct {
	sent chan string
}

func (m *mailbox) Send(_ context.Context, _, _, body string) error {
	m.sent <- body
	return nil
}

func (m *mailbox) next(t *testing.T) string {
	t.Helper()
	select {
	case body := <-m.sent:
		return body
	case <-time.After(2 * time.Second):
		t.Fatal("no email was sent")
		return ""
	}
}

func (m *mailbox) none(t *testing.T) {
	t.Helper()
	select {
	case body := <-m.sent:
		t.Fatalf("unexpected email: %q", body)
	case <-time.After(200 * time.Millisecond):
	}
}

// FR-004, SC-004, through the service and the real store: one email per quarter hour and address, with a new link.
func TestSubscribe_OneConfirmationPerQuarterHourAndAddress(t *testing.T) {
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })

	db := openTestDB(t)
	mail := &mailbox{sent: make(chan string, 8)}
	svc := status.NewSubscriberService(NewSubscriberStore(db), mail, "http://status.example.com", testLogger())
	ctx := context.Background()
	const email = "visitor@example.com"

	require.NoError(t, svc.Subscribe(ctx, email))
	first := mail.next(t)
	require.NoError(t, svc.Subscribe(ctx, email))
	mail.none(t)

	backdateSubscriber(t, db, email, 16*time.Minute)
	require.NoError(t, svc.Subscribe(ctx, email))
	second := mail.next(t)
	assert.NotEqual(t, first, second, "a new confirmation carries a new link")
	require.NoError(t, svc.Subscribe(ctx, "other@example.com"))
	mail.next(t)
}
