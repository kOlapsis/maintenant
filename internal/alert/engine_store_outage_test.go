// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func (f *storeErrorFixture) testRows(t *testing.T, entityID string) int {
	t.Helper()
	rows, err := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: "test"})
	require.NoError(t, err)
	n := 0
	for _, a := range rows {
		if a.EntityID == entityID {
			n++
		}
	}
	return n
}

// secondChannel adds a channel to the fixture's trigger and returns the ids of the first and of the new one.
func (f *storeErrorFixture) secondChannel(t *testing.T) (string, string) {
	t.Helper()
	id, err := f.channels.InsertChannel(f.ctx, &alert.NotificationChannel{Name: "hook-b", Type: "webhook", URL: "https://example.com/b", Enabled: true})
	require.NoError(t, err)
	first := f.trigger.ChannelIDs[0]
	f.trigger.ChannelIDs = []string{first, id}
	require.NoError(t, f.triggers.UpdateTrigger(f.ctx, f.trigger))
	return first, id
}

// events lists, in order, the event types an alert's deliveries to one channel carry.
func (f *storeErrorFixture) events(t *testing.T, alertID, channelID string) []string {
	t.Helper()
	ds, err := f.channels.ListDeliveriesByAlert(f.ctx, alertID)
	require.NoError(t, err)
	var out []string
	for _, d := range ds {
		if d.ChannelID == channelID {
			out = append(out, d.EventType)
		}
	}
	return out
}

func (f *storeErrorFixture) errorLines() []string {
	var out []string
	for _, line := range strings.Split(f.logs.String(), "\n") {
		if strings.Contains(line, "level=ERROR") {
			out = append(out, line)
		}
	}
	return out
}

func TestEngineStoreOutage_AlertStoredOnceTheStoreIsBack(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.fail.set("InsertAlert", -1)
	f.send(outageEvent("ep-40", alert.SeverityCritical, false))
	f.barrier(t)
	require.Empty(t, f.activeRows("ep-40"))

	f.fail.set("InsertAlert", 0)
	f.engine.RetryStoreFailures(f.ctx)
	rows := f.activeRows("ep-40")
	require.Len(t, rows, 1, "an alert the store refused must be stored once the store is back")
	assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "and notified")
	assert.Equal(t, 1, f.inMemory())
}

func TestEngineStoreOutage_RecoveryOfAnUnstoredAlert(t *testing.T) {
	t.Run("store back before the recovery", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-41", alert.SeverityCritical, false))
		f.barrier(t)

		f.fail.set("InsertAlert", 0)
		f.send(outageEvent("ep-41", alert.SeverityInfo, true))
		f.barrier(t)

		rows, err := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: "test", Status: alert.StatusResolved})
		require.NoError(t, err)
		var original *alert.Alert
		for _, a := range rows {
			if a.EntityID == "ep-41" && a.ResolvedByID != nil {
				original = a
			}
		}
		require.NotNil(t, original, "the outage must be stored, then resolved by its recovery")
		assert.Equal(t, 2, f.deliveries(t, original.ID), "the outage and its recovery must each be notified")
		assert.Equal(t, 0, f.inMemory())
	})

	t.Run("store still down at the recovery", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-42", alert.SeverityCritical, false))
		f.barrier(t)
		f.send(outageEvent("ep-42", alert.SeverityInfo, true))
		f.barrier(t)

		f.fail.set("InsertAlert", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Zero(t, f.testRows(t, "ep-42"), "an outage over before it could be stored must not be raised later")
	})

	t.Run("entity removed before the store is back", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-45", alert.SeverityCritical, false))
		f.barrier(t)
		f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-45")

		f.fail.set("InsertAlert", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Zero(t, f.testRows(t, "ep-45"), "a removed entity must not be raised later")
	})
}

func TestEngineStoreOutage_NotifiedOnceTheRoutingIsReadable(t *testing.T) {
	for _, method := range []string{"ListEnabledTriggers", "GetChannel"} {
		t.Run(method, func(t *testing.T) {
			f := newStoreErrorFixture(t)
			f.engine.Start(f.ctx)

			f.fail.set(method, -1)
			f.send(outageEvent("ep-43", alert.SeverityCritical, false))
			f.barrier(t)
			rows := f.activeRows("ep-43")
			require.Len(t, rows, 1)
			require.Zero(t, f.deliveries(t, rows[0].ID))

			f.fail.set(method, 0)
			f.engine.RetryStoreFailures(f.ctx)
			assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "an alert whose routing could not be read must be notified once it can")
			f.engine.RetryStoreFailures(f.ctx)
			assert.Equal(t, 1, f.deliveries(t, rows[0].ID), "the alert must be notified once")
		})
	}
}

func TestEngineStoreOutage_UnreadableChannelHoldsOnlyItself(t *testing.T) {
	f := newStoreErrorFixture(t)
	a, b := f.secondChannel(t)
	f.engine.Start(f.ctx)

	f.fail.set("GetChannel:"+b, -1)
	f.send(outageEvent("ep-50", alert.SeverityCritical, false))
	f.barrier(t)
	original := f.waitActive(t, "ep-50")
	require.Equal(t, []string{alert.EventFired}, f.events(t, original.ID, a))

	f.send(outageEvent("ep-50", alert.SeverityInfo, true))
	f.barrier(t)
	assert.Equal(t, []string{alert.EventFired, alert.EventResolved}, f.events(t, original.ID, a),
		"the readable channel must hear about the recovery")

	f.fail.set("GetChannel:"+b, 0)
	f.engine.RetryStoreFailures(f.ctx)
	assert.Equal(t, []string{alert.EventResolved}, f.events(t, original.ID, b),
		"a channel readable only after the recovery gets the recovery, never the firing")
}

func TestEngineStoreOutage_StuckAlertDoesNotHoldOthers(t *testing.T) {
	f := newStoreErrorFixture(t)
	a, b := f.secondChannel(t)
	f.engine.Start(f.ctx)

	f.fail.set("GetChannel:"+b, -1)
	f.send(outageEvent("ep-51", alert.SeverityCritical, false))
	f.barrier(t)

	f.fail.set("ListEnabledTriggers", -1)
	f.send(outageEvent("ep-52", alert.SeverityCritical, false))
	f.barrier(t)
	second := f.waitActive(t, "ep-52")
	require.Empty(t, f.events(t, second.ID, a))

	f.fail.set("ListEnabledTriggers", 0)
	f.engine.RetryStoreFailures(f.ctx)
	assert.Equal(t, []string{alert.EventFired}, f.events(t, second.ID, a),
		"an alert queued behind one stuck on an unreadable channel must still be routed")
}

func TestEngineStoreOutage_UnreadableChannelGivenUpAfterADay(t *testing.T) {
	f := newStoreErrorFixture(t)
	_, b := f.secondChannel(t)
	f.engine.Start(f.ctx)

	f.fail.set("GetChannel:"+b, -1)
	f.send(outageEvent("ep-53", alert.SeverityCritical, false))
	f.barrier(t)
	original := f.waitActive(t, "ep-53")
	f.engine.RetryStoreFailures(f.ctx)
	require.Equal(t, 1, f.engine.Unrouted())

	f.clock.Advance(24*time.Hour + time.Second)
	f.engine.RetryStoreFailures(f.ctx)
	assert.Zero(t, f.engine.Unrouted(), "a notification still unroutable after a day must be dropped")
	errs := f.errorLines()
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[len(errs)-1], original.ID)
	assert.Contains(t, errs[len(errs)-1], b)

	reads := f.fail.count("GetChannel:" + b)
	f.engine.RetryStoreFailures(f.ctx)
	assert.Equal(t, reads, f.fail.count("GetChannel:"+b), "a dropped notification must not be read again")
}

func TestEngineStoreOutage_CeilingOnWaitingNotifications(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.SetMaxUnrouted(2)
	f.engine.Start(f.ctx)

	f.fail.set("ListEnabledTriggers", -1)
	for _, ep := range []string{"ep-54", "ep-55", "ep-56"} {
		f.send(outageEvent(ep, alert.SeverityCritical, false))
	}
	f.barrier(t)
	assert.Equal(t, 2, f.engine.Unrouted())
	dropped := f.waitActive(t, "ep-56")
	assert.Condition(t, func() bool {
		for _, line := range f.errorLines() {
			if strings.Contains(line, dropped.ID) {
				return true
			}
		}
		return false
	}, "the notification over the ceiling must be logged as dropped")

	f.fail.set("ListEnabledTriggers", 0)
	f.engine.RetryStoreFailures(f.ctx)
	for _, ep := range []string{"ep-54", "ep-55"} {
		assert.Equal(t, 1, f.deliveries(t, f.waitActive(t, ep).ID), "%s was kept and must be notified", ep)
	}
	assert.Zero(t, f.deliveries(t, dropped.ID))
}

func TestEngineStoreOutage_RetriedNotificationFollowsTheAlert(t *testing.T) {
	t.Run("acknowledged meanwhile", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("ListEnabledTriggers", -1)
		f.send(outageEvent("ep-57", alert.SeverityCritical, false))
		f.barrier(t)
		original := f.waitActive(t, "ep-57")
		_, err := f.engine.Acknowledge(f.ctx, original.ID, "alice")
		require.NoError(t, err)

		f.fail.set("ListEnabledTriggers", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Zero(t, f.deliveries(t, original.ID), "an acknowledged alert must not be notified as firing")
		assert.Zero(t, f.engine.Unrouted())
	})

	t.Run("silenced meanwhile", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("ListEnabledTriggers", -1)
		f.send(outageEvent("ep-58", alert.SeverityCritical, false))
		f.barrier(t)
		original := f.waitActive(t, "ep-58")
		entity := "ep-58"
		ruleID, err := f.silences.InsertSilenceRule(f.ctx, &alert.SilenceRule{
			Source: "test", EntityType: "endpoint", EntityID: &entity, StartsAt: time.Now().Add(-time.Minute), DurationSeconds: 3600,
		})
		require.NoError(t, err)

		f.fail.set("ListEnabledTriggers", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Zero(t, f.deliveries(t, original.ID), "a firing must wait while a silence covers the alert")

		require.NoError(t, f.silences.CancelSilenceRule(f.ctx, ruleID))
		f.engine.RetryStoreFailures(f.ctx)
		assert.Equal(t, []string{alert.EventFired}, f.events(t, original.ID, f.trigger.ChannelIDs[0]),
			"the firing must go out once the silence ends")
	})

	t.Run("entity removed meanwhile", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("ListEnabledTriggers", -1)
		f.send(outageEvent("ep-59", alert.SeverityCritical, false))
		f.barrier(t)
		original := f.waitActive(t, "ep-59")
		f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-59")

		f.fail.set("ListEnabledTriggers", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Zero(t, f.deliveries(t, original.ID), "a removed entity's alert must not be notified")
	})

	t.Run("resolved meanwhile", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.Start(f.ctx)

		f.fail.set("ListEnabledTriggers", -1)
		f.send(outageEvent("ep-60", alert.SeverityCritical, false))
		f.barrier(t)
		original := f.waitActive(t, "ep-60")
		f.send(outageEvent("ep-60", alert.SeverityInfo, true))
		f.barrier(t)

		f.fail.set("ListEnabledTriggers", 0)
		f.engine.RetryStoreFailures(f.ctx)
		assert.Equal(t, []string{alert.EventResolved}, f.events(t, original.ID, f.trigger.ChannelIDs[0]),
			"an alert resolved before it could be routed must only be notified as resolved")
	})
}
