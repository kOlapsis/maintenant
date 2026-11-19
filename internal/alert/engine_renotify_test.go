// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func reminder(entityID, severity string, days int) alert.Event {
	evt := outageEvent(entityID, severity, false)
	evt.Renotify = true
	evt.Message = fmt.Sprintf("%s expires in %d days", entityID, days)
	return evt
}

// notified drains the notifier and returns the messages sent for one alert, in order.
func (f *silenceFixture) notified(id string) []string {
	var out []string
	for _, a := range f.notifier.QueuedAlerts() {
		if a.ID == id {
			out = append(out, a.Message)
		}
	}
	return out
}

func (f *silenceFixture) live(t *testing.T, entityID string) *alert.Alert {
	t.Helper()
	rows := f.rows(t, "test", entityID)
	require.Len(t, rows, 1)
	require.Equal(t, alert.StatusActive, rows[0].Status)
	return rows[0]
}

func (f *silenceFixture) stored(t *testing.T, id string) *alert.Alert {
	t.Helper()
	a, err := f.alerts.GetAlert(f.ctx, id)
	require.NoError(t, err)
	require.NotNil(t, a)
	return a
}

func TestRenotify_ReminderAtTheSameSeverityIsNotified(t *testing.T) {
	f := newSilenceFixture(t)
	f.start(time.Hour)

	f.send(reminder("cert-1", alert.SeverityCritical, 7))
	f.barrier(t)
	id := f.live(t, "cert-1").ID

	f.send(reminder("cert-1", alert.SeverityCritical, 3))
	f.barrier(t)

	assert.Equal(t, 2, f.deliveries(t, id), "the reminder must be notified")
	assert.Equal(t, "cert-1 expires in 3 days", f.stored(t, id).Message)
	assert.Equal(t, []string{"cert-1 expires in 7 days", "cert-1 expires in 3 days"}, f.notified(id),
		"a reminder at the same severity must not read as a raise")
	assert.Equal(t, 1, f.escalator.count(id), "a reminder must not restart escalation")
}

func TestRenotify_ReminderThatRaisesSaysSo(t *testing.T) {
	f := newSilenceFixture(t)
	f.start(time.Hour)

	f.send(reminder("cert-2", alert.SeverityWarning, 14))
	f.barrier(t)
	id := f.live(t, "cert-2").ID

	f.send(reminder("cert-2", alert.SeverityCritical, 7))
	f.barrier(t)

	assert.Equal(t, []string{
		"cert-2 expires in 14 days",
		"Severity raised from warning to critical: cert-2 expires in 7 days",
	}, f.notified(id))
	assert.Equal(t, 2, f.escalator.count(id), "a raise may match policies the lower severity did not")
}

func TestRenotify_AcknowledgedAlertIsRefreshedNotNotified(t *testing.T) {
	f := newSilenceFixture(t)
	f.start(time.Hour)

	f.send(reminder("cert-3", alert.SeverityCritical, 7))
	f.barrier(t)
	id := f.live(t, "cert-3").ID
	_, err := f.engine.Acknowledge(f.ctx, id, "alice")
	require.NoError(t, err)

	f.send(reminder("cert-3", alert.SeverityCritical, 3))
	f.barrier(t)

	got := f.stored(t, id)
	assert.Equal(t, "cert-3 expires in 3 days", got.Message, "the acknowledged alert must still show the latest reminder")
	assert.Equal(t, "alice", got.AcknowledgedBy)
	assert.Equal(t, 1, f.deliveries(t, id), "an acknowledged alert must not be notified again")
}

func TestRenotify_HeldAlertTakesTheLatestReminder(t *testing.T) {
	f := newSilenceFixture(t)
	f.silenceEndpoint(t, "cert-4")
	f.start(fastRecheck)

	f.send(reminder("cert-4", alert.SeverityCritical, 7))
	f.send(reminder("cert-4", alert.SeverityCritical, 3))
	f.barrier(t)
	held := f.held(t, "cert-4")
	assert.Equal(t, "cert-4 expires in 3 days", held.Message)

	f.elapse(t, 2*time.Hour)
	f.assertPromotedOnce(t, held.ID)
	assert.Equal(t, []string{"cert-4 expires in 3 days"}, f.notified(held.ID))
}
