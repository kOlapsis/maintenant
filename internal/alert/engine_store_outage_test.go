// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

const outageRecheck = 10 * time.Millisecond

func (f *storeErrorFixture) deliveriesNow(alertID string) int {
	ds, _ := f.channels.ListDeliveriesByAlert(f.ctx, alertID)
	return len(ds)
}

func (f *storeErrorFixture) testRowsNow(entityID string) int {
	rows, _ := f.alerts.ListAlerts(f.ctx, alert.ListAlertsOpts{Source: "test"})
	n := 0
	for _, a := range rows {
		if a.EntityID == entityID {
			n++
		}
	}
	return n
}

func TestEngineStoreOutage_AlertStoredOnceTheStoreIsBack(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.SetSilenceRecheckInterval(outageRecheck)
	f.engine.Start(f.ctx)

	f.fail.set("InsertAlert", -1)
	f.send(outageEvent("ep-40", alert.SeverityCritical, false))
	f.barrier(t)
	require.Empty(t, f.activeRows("ep-40"))

	f.fail.set("InsertAlert", 0)
	require.Eventually(t, func() bool {
		rows := f.activeRows("ep-40")
		return len(rows) == 1 && f.deliveriesNow(rows[0].ID) == 1
	}, 10*time.Second, 5*time.Millisecond, "an alert the store refused must be stored and notified once the store is back")
	f.engine.Settle()
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
		f.engine.SetSilenceRecheckInterval(outageRecheck)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-42", alert.SeverityCritical, false))
		f.barrier(t)
		f.send(outageEvent("ep-42", alert.SeverityInfo, true))
		f.barrier(t)

		f.fail.set("InsertAlert", 0)
		assert.Never(t, func() bool { return f.testRowsNow("ep-42") > 0 }, 200*time.Millisecond, outageRecheck,
			"an outage over before it could be stored must not be raised later")
	})

	t.Run("entity removed before the store is back", func(t *testing.T) {
		f := newStoreErrorFixture(t)
		f.engine.SetSilenceRecheckInterval(outageRecheck)
		f.engine.Start(f.ctx)

		f.fail.set("InsertAlert", -1)
		f.send(outageEvent("ep-45", alert.SeverityCritical, false))
		f.barrier(t)
		f.engine.ResolveByEntity(f.ctx, "endpoint", "ep-45")

		f.fail.set("InsertAlert", 0)
		assert.Never(t, func() bool { return f.testRowsNow("ep-45") > 0 }, 200*time.Millisecond, outageRecheck,
			"a removed entity must not be raised later")
	})
}

func TestEngineStoreOutage_NotifiedOnceTheRoutingIsReadable(t *testing.T) {
	for _, method := range []string{"ListEnabledTriggers", "GetChannel"} {
		t.Run(method, func(t *testing.T) {
			f := newStoreErrorFixture(t)
			f.engine.SetSilenceRecheckInterval(outageRecheck)
			f.engine.Start(f.ctx)

			f.fail.set(method, -1)
			f.send(outageEvent("ep-43", alert.SeverityCritical, false))
			f.barrier(t)
			rows := f.activeRows("ep-43")
			require.Len(t, rows, 1)
			require.Zero(t, f.deliveries(t, rows[0].ID))

			f.fail.set(method, 0)
			require.Eventually(t, func() bool { return f.deliveriesNow(rows[0].ID) == 1 }, 10*time.Second, 5*time.Millisecond,
				"an alert whose routing could not be read must be notified once it can")
			f.engine.Settle()
			assert.Never(t, func() bool { return f.deliveriesNow(rows[0].ID) > 1 }, 200*time.Millisecond, outageRecheck,
				"the alert must be notified once")
		})
	}
}

func TestEngineStoreOutage_RecoveryFollowsTheAlertItResolves(t *testing.T) {
	f := newStoreErrorFixture(t)
	f.engine.Start(f.ctx)

	f.fail.set("ListEnabledTriggers", -1)
	f.send(outageEvent("ep-44", alert.SeverityCritical, false))
	f.barrier(t)
	original := f.waitActive(t, "ep-44")

	f.fail.set("ListEnabledTriggers", 0)
	f.send(outageEvent("ep-44", alert.SeverityInfo, true))
	f.barrier(t)

	ds, err := f.channels.ListDeliveriesByAlert(f.ctx, original.ID)
	require.NoError(t, err)
	require.Len(t, ds, 2, "the outage must be notified before its recovery")
	assert.Equal(t, alert.EventFired, ds[0].EventType)
	assert.Equal(t, alert.EventResolved, ds[1].EventType)
}
