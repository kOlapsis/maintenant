// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/webhook"
)

func createTestWebhook(t *testing.T, ws *WebhookStoreImpl, id string) {
	t.Helper()
	require.NoError(t, ws.Create(context.Background(), &webhook.WebhookSubscription{
		ID: id, Name: id, URL: "https://hooks.example.com/" + id,
		EventTypes: []string{"*"}, IsActive: true, CreatedAt: time.Now(),
	}))
}

func TestWebhookStore_CreateStoresEpochTimestamps(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	ws := NewWebhookStore(db)

	createdAt := time.Now().Add(-time.Hour)
	require.NoError(t, ws.Create(ctx, &webhook.WebhookSubscription{
		ID: "w1", Name: "hook", URL: "https://hooks.example.com/w1",
		EventTypes: []string{"*"}, IsActive: true, CreatedAt: createdAt,
	}))

	got, err := ws.GetByID(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, createdAt.Unix(), got.CreatedAt.Unix())
	assert.Nil(t, got.LastDeliveryAt)
}

func TestWebhookStore_RecordDeliveryDisablesAfterConsecutiveFailures(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	ws := NewWebhookStore(db)
	createTestWebhook(t, ws, "w1")

	for i := 1; i < webhook.MaxConsecutiveFailures; i++ {
		require.NoError(t, ws.RecordDelivery(ctx, "w1", false))
	}
	sub, err := ws.GetByID(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, webhook.MaxConsecutiveFailures-1, sub.FailureCount)
	assert.True(t, sub.IsActive)
	require.NotNil(t, sub.LastDeliveryStatus)
	assert.Equal(t, webhook.DeliveryFailed, *sub.LastDeliveryStatus)
	require.NotNil(t, sub.LastDeliveryAt)
	assert.WithinDuration(t, time.Now(), *sub.LastDeliveryAt, time.Minute)

	require.NoError(t, ws.RecordDelivery(ctx, "w1", false))
	sub, err = ws.GetByID(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, webhook.MaxConsecutiveFailures, sub.FailureCount)
	assert.False(t, sub.IsActive, "the tenth failure in a row disables the webhook")
	active, err := ws.ListActive(ctx)
	require.NoError(t, err)
	assert.Empty(t, active)

	require.NoError(t, ws.RecordDelivery(ctx, "w1", true))
	sub, err = ws.GetByID(ctx, "w1")
	require.NoError(t, err)
	assert.Zero(t, sub.FailureCount, "a success resets the count")
	assert.True(t, sub.IsActive, "a success re-enables the webhook")
	assert.Equal(t, webhook.DeliveryDelivered, *sub.LastDeliveryStatus)
}

func TestWebhookStore_ReadsRFC3339TimestampsOfEarlierBuilds(t *testing.T) {
	requireSQLite(t)
	db := openTestDB(t)
	ctx := context.Background()
	ws := NewWebhookStore(db)

	_, err := db.Writer().Exec(ctx,
		`INSERT INTO webhook_subscriptions (id, name, url, event_types, last_delivery_status, last_delivery_at, created_at)
		 VALUES ('legacy', 'legacy', 'https://hooks.example.com/legacy', '["*"]', 'delivered', '2026-05-01T10:00:00Z', '2026-04-01T09:00:00Z')`)
	require.NoError(t, err)

	subs, err := ws.List(ctx)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), subs[0].CreatedAt.UTC())
	require.NotNil(t, subs[0].LastDeliveryAt)
	assert.Equal(t, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), subs[0].LastDeliveryAt.UTC())
}
