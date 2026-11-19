// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
)

func seedDeliveryStream(t *testing.T, db *DB) (channels *ChannelStoreImpl, alertID string, channelID string) {
	t.Helper()
	ctx := context.Background()
	channels = NewChannelStore(db)
	channelID, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "hook", Type: "webhook", URL: "https://example.com/hook", Enabled: true})
	require.NoError(t, err)
	alertID, err = NewAlertStore(db).InsertAlert(ctx, &alert.Alert{
		Source: "test", AlertType: "down", Severity: alert.SeverityCritical, Status: alert.StatusActive, Message: "api down",
		EntityType: "endpoint", EntityID: "e1", EntityName: "api", AgentID: uid.LocalAgent, FiredAt: time.Now(),
	})
	require.NoError(t, err)
	return channels, alertID, channelID
}

func insertPending(t *testing.T, channels *ChannelStoreImpl, alertID, channelID, eventType string) *alert.NotificationDelivery {
	t.Helper()
	d := &alert.NotificationDelivery{
		AlertID: alertID, ChannelID: channelID, EventType: eventType, Status: alert.DeliveryPending,
		Payload: &alert.Alert{ID: alertID, Status: alert.StatusActive, Message: "Severity raised from warning to critical: api down"},
	}
	id, err := channels.InsertDelivery(context.Background(), d)
	require.NoError(t, err)
	require.Equal(t, id, d.ID)
	return d
}

func dueIDs(t *testing.T, channels *ChannelStoreImpl, now time.Time) []string {
	t.Helper()
	due, err := channels.ListDueDeliveries(context.Background(), now, 10)
	require.NoError(t, err)
	ids := []string{}
	for _, d := range due {
		ids = append(ids, d.ID)
	}
	return ids
}

func TestDeliveryQueue_AClaimTakesTheAttemptOnce(t *testing.T) {
	ctx := context.Background()
	channels, alertID, channelID := seedDeliveryStream(t, openTestDB(t))
	d := insertPending(t, channels, alertID, channelID, alert.EventSeverityRaised)

	due, err := channels.ListDueDeliveries(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, due, 1, "a new delivery is due at once")
	assert.Equal(t, alert.EventSeverityRaised, due[0].EventType)
	assert.Equal(t, d.NextAttemptAt, due[0].NextAttemptAt)
	require.NotNil(t, due[0].Payload, "the notification is rebuilt from the row")
	assert.Equal(t, "Severity raised from warning to critical: api down", due[0].Payload.Message)

	var wins, failures atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen := *due[0]
			ok, err := channels.ClaimDelivery(ctx, &seen, time.Now().Add(2*time.Minute))
			if err != nil {
				failures.Add(1)
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Zero(t, failures.Load())
	assert.EqualValues(t, 1, wins.Load(), "one worker takes the attempt, the others see it gone")

	assert.Empty(t, dueIDs(t, channels, time.Now()), "a claimed attempt is not due while it is being sent")
	assert.Equal(t, []string{d.ID}, dueIDs(t, channels, time.Now().Add(3*time.Minute)), "it is due again if its sender never reports")
}

func TestDeliveryQueue_ARetryIsDueAtItsNextAttempt(t *testing.T) {
	ctx := context.Background()
	channels, alertID, channelID := seedDeliveryStream(t, openTestDB(t))
	d := insertPending(t, channels, alertID, channelID, alert.EventFired)

	next := time.Now().Add(time.Hour).Truncate(time.Second)
	d.Attempts, d.LastError, d.NextAttemptAt = 1, "non-2xx response: 503", next
	require.NoError(t, channels.UpdateDelivery(ctx, d))

	assert.Empty(t, dueIDs(t, channels, time.Now()))
	assert.Equal(t, []string{d.ID}, dueIDs(t, channels, next))

	stored, err := channels.ListDeliveriesByAlert(ctx, alertID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, alert.DeliveryPending, stored[0].Status)
	assert.Equal(t, alert.EventFired, stored[0].EventType)
	assert.Equal(t, next, stored[0].NextAttemptAt)
	assert.Equal(t, 1, stored[0].Attempts)
}

func TestDeliveryQueue_ADeliveryWaitsForTheEarlierOneOfItsStream(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	channels, alertID, channelID := seedDeliveryStream(t, db)
	otherChannel, err := channels.InsertChannel(ctx, &alert.NotificationChannel{Name: "other", Type: "webhook", URL: "https://example.com/other", Enabled: true})
	require.NoError(t, err)

	fired := insertPending(t, channels, alertID, channelID, alert.EventFired)
	resolved := insertPending(t, channels, alertID, channelID, alert.EventResolved)
	elsewhere := insertPending(t, channels, alertID, otherChannel, alert.EventFired)

	assert.ElementsMatch(t, []string{fired.ID, elsewhere.ID}, dueIDs(t, channels, time.Now()),
		"the recovery waits for the alert it resolves on the same channel, and only there")
	seen := *resolved
	ok, err := channels.ClaimDelivery(ctx, &seen, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, ok, "the recovery cannot be taken before the alert is out")

	fired.Status, fired.Attempts = alert.DeliveryDelivered, 1
	require.NoError(t, channels.UpdateDelivery(ctx, fired))

	assert.ElementsMatch(t, []string{resolved.ID, elsewhere.ID}, dueIDs(t, channels, time.Now()))
	ok, err = channels.ClaimDelivery(ctx, resolved, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, ok)
}

func readMigration(t *testing.T, version, direction string) string {
	t.Helper()
	matches, err := fs.Glob(migrationFS, "migrations/sqlite/"+version+"_*."+direction+".sql")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	body, err := fs.ReadFile(migrationFS, matches[0])
	require.NoError(t, err)
	return string(body)
}

func TestMigration47_DownThenUpOnSQLiteWithRows(t *testing.T) {
	ctx := context.Background()
	db := openTestSQLite(t)
	channels, alertID, channelID := seedDeliveryStream(t, db)
	delivered := insertPending(t, channels, alertID, channelID, alert.EventFired)
	delivered.Status, delivered.Attempts = alert.DeliveryDelivered, 1
	require.NoError(t, channels.UpdateDelivery(ctx, delivered))
	insertPending(t, channels, alertID, channelID, alert.EventResolved)

	_, err := db.ReadDB().ExecContext(ctx, readMigration(t, "47", "down"))
	require.NoError(t, err)
	_, err = db.ReadDB().ExecContext(ctx, `SELECT next_attempt_at FROM notification_deliveries`)
	require.Error(t, err, "the down migration drops the queue columns")
	var rows int
	require.NoError(t, db.ReadDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries`).Scan(&rows))
	assert.Equal(t, 2, rows, "and keeps the deliveries")

	// What the previous version leaves behind: a delivery dropped from its queue, pending forever.
	now := time.Now().Unix()
	_, err = db.ReadDB().ExecContext(ctx, `INSERT INTO notification_deliveries (id, alert_id, channel_id, status, attempts, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', 0, ?, ?)`, uid.New(), alertID, channelID, now, now)
	require.NoError(t, err)

	_, err = db.ReadDB().ExecContext(ctx, readMigration(t, "47", "up"))
	require.NoError(t, err)

	stored, err := channels.ListDeliveriesByAlert(ctx, alertID)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	statuses := map[string]int{}
	for _, d := range stored {
		statuses[d.Status]++
		assert.Equal(t, "", d.EventType)
		assert.Nil(t, d.Payload)
		if d.Status == alert.DeliveryFailed {
			assert.Equal(t, "abandoned: left pending by a previous version", d.LastError)
		}
	}
	assert.Equal(t, map[string]int{alert.DeliveryDelivered: 1, alert.DeliveryFailed: 2}, statuses,
		"a pending row of the previous version cannot be rebuilt, it is closed instead of sent")
	assert.Empty(t, dueIDs(t, channels, time.Now()))
}
