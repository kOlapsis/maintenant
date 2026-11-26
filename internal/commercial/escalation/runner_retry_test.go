// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
)

func policyOf(levels ...esc.Level) *esc.Policy {
	return &esc.Policy{
		Name: "p", Active: true,
		Filters: esc.Filters{Severities: []string{alert.SeverityCritical}},
		Levels:  levels,
	}
}

// fireFirstLevel starts the run of a and runs the evaluation at which its first level, due after 60 s, goes out.
func (h *harness) fireFirstLevel(t *testing.T, a *alert.Alert) {
	t.Helper()
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.settle()
}

func (h *harness) levelDelivery(t *testing.T, level int, channelID string) *esc.Delivery {
	t.Helper()
	for _, d := range h.store.listDeliveries() {
		if d.LevelIndex == level && d.ChannelID != nil && *d.ChannelID == channelID {
			return d
		}
	}
	require.FailNow(t, "no delivery", "level %d to channel %s", level, channelID)
	return nil
}

func count(messages []string, message string) int {
	n := 0
	for _, m := range messages {
		if m == message {
			n++
		}
	}
	return n
}

func TestRetry_OneLevelPagesOnceTheChannelIsBack(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyOf(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{"1"}}))

	h.sender.down("1", true)
	h.fireFirstLevel(t, a)
	h.minutes(t, 3)
	require.Empty(t, h.sender.delivered("1"))

	h.sender.down("1", false)
	h.minutes(t, 15)
	assert.Equal(t, []string{a.Message, formatExhaustedAlert(a, 1).Message}, h.sender.delivered("1"),
		"the page goes out once the channel is back, then the exhaustion notice")
	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, esc.RunStatusExhausted, runs[0].Status)
}

func TestRetry_NextLevelKeepsItsTimeWhileAnEarlierOneRetries(t *testing.T) {
	h := newHarness(t)
	h.channels.put(&alert.NotificationChannel{ID: "a", Name: "first", Type: "slack", URL: "ua", Enabled: true})
	h.channels.put(&alert.NotificationChannel{ID: "b", Name: "second", Type: "slack", URL: "ub", Enabled: true})
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyOf(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{"a"}},
		esc.Level{Order: 1, DelaySeconds: 180, ChannelIDs: []string{"b"}},
	))

	h.sender.down("a", true)
	h.fireFirstLevel(t, a)
	h.minutes(t, 2)
	assert.Equal(t, []string{a.Message}, h.sender.delivered("b"), "the second level goes out at its delay")
	assert.Empty(t, h.sender.delivered("a"))

	h.minutes(t, 8)
	h.sender.down("a", false)
	h.minutes(t, 15)
	assert.Equal(t, []string{a.Message}, h.sender.delivered("a"), "the first level's page arrives once its channel is back")
}

func TestRetry_StopsWhenTheAlertResolves(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyOf(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{"1"}}))

	h.sender.down("1", true)
	h.fireFirstLevel(t, a)
	h.minutes(t, 1)
	resolved := *a
	resolved.Status = alert.StatusResolved
	h.alerts.put(&resolved)
	require.NoError(t, h.runner.OnAlertResolved(context.Background(), a.ID, h.clock()))

	h.sender.down("1", false)
	h.minutes(t, 10)
	assert.Empty(t, h.sender.delivered("1"), "nothing goes out once the alert is resolved")
	assert.Equal(t, esc.DeliveryStatusAbandoned, h.levelDelivery(t, 0, "1").Status)
}

func TestRetry_StopsWhenTheAlertIsAcknowledged(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())

	h.sender.down("1", true)
	h.fireFirstLevel(t, a)
	acked := *a
	at := h.clock()
	acked.AcknowledgedAt, acked.AcknowledgedBy = &at, "alice"
	h.alerts.put(&acked)
	ack := alert.Acknowledgment{By: "alice", At: at}
	require.NoError(t, h.runner.OnAlertAcknowledged(context.Background(), a.ID, ack))
	h.settle()

	h.sender.down("1", false)
	h.minutes(t, 10)
	assert.Equal(t, []string{formatAckAlert(a, ack).Message}, h.sender.delivered("1"),
		"the page is dropped once the alert is acknowledged, the acknowledgment notice still goes out")
	assert.Equal(t, esc.DeliveryStatusAbandoned, h.levelDelivery(t, 0, "1").Status)
}

func TestRetry_APermanentRefusalFailsAtOnce(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())

	h.sender.refuse("1", &alert.HTTPStatusError{Status: http.StatusForbidden})
	h.fireFirstLevel(t, a)
	d := h.levelDelivery(t, 0, "1")
	assert.Equal(t, esc.DeliveryStatusFailed, d.Status)
	assert.Equal(t, "non-2xx response: 403", d.Error)

	h.minutes(t, 1)
	pages := 0
	for _, c := range h.sender.snapshot() {
		if c.Message == a.Message {
			pages++
		}
	}
	assert.Equal(t, 1, pages, "a refused page is not tried again")
}

func TestRetry_ARestartResumesThePendingRetryOnce(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))

	sender.down(ch, true)
	h.advance(61 * time.Second)
	h.cycle()
	h.settle()
	ds := h.deliveries(h.onlyRun(a).ID)
	require.Len(t, ds, 1)
	require.Equal(t, esc.DeliveryStatusPending, ds[0].Status, "a page that could not go out waits for its retry")

	h.restart(sender)
	sender.down(ch, false)
	for range 3 {
		h.advance(time.Minute)
		h.cycle()
		h.settle()
	}
	assert.Equal(t, 1, count(sender.delivered(ch), a.Message), "the restarted runner sends the page once")
}

func (h *dbHarness) pass(step time.Duration, n int) {
	h.t.Helper()
	for range n {
		h.advance(step)
		h.cycle()
		h.settle()
	}
}

func pageAttempts(sender *fakeSender, a *alert.Alert) int {
	n := 0
	for _, c := range sender.snapshot() {
		if c.Message == a.Message {
			n++
		}
	}
	return n
}

func TestRetry_AnAttemptCutShortIsTakenAgainOnceItsClaimEnds(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)
	_, err := h.store.Store.InsertDelivery(h.ctx, &esc.Delivery{
		RunID: h.onlyRun(a).ID, LevelIndex: 0, ChannelID: &ch, Status: esc.DeliveryStatusPending,
		AttemptStartedAt: h.clock(), Attempts: 1, NextAttemptAt: h.clock().Add(time.Hour),
	})
	require.NoError(t, err, "the page as a server stopped in the middle of its first attempt leaves it")

	h.restart(sender)
	h.pass(59*time.Minute, 1)
	assert.Zero(t, pageAttempts(sender, a), "an attempt is not taken again while its claim holds")

	h.pass(2*time.Minute, 3)
	assert.Equal(t, 1, pageAttempts(sender, a), "it is taken again once, after its claim")
	assert.Equal(t, 1, count(sender.delivered(ch), a.Message))
}

func TestRetry_APageNotDeliveredWithinADayFails(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))

	sender.down(ch, true)
	h.pass(61*time.Second, 1)
	h.pass(time.Hour, 25)
	var page *esc.Delivery
	for _, d := range h.deliveries(h.onlyRun(a).ID) {
		if d.LevelIndex == 0 {
			page = d
		}
	}
	require.NotNil(t, page)
	assert.Equal(t, esc.DeliveryStatusFailed, page.Status, "a page not delivered within 24 hours is given up")

	attempts := pageAttempts(sender, a)
	h.pass(time.Hour, 3)
	assert.Equal(t, attempts, pageAttempts(sender, a), "a page given up is not tried again")
}

func TestRetry_AMaintenanceWindowHoldsTheRetries(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}},
		esc.Level{Order: 1, DelaySeconds: 3600, ChannelIDs: []string{ch}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))

	sender.down(ch, true)
	h.pass(61*time.Second, 1)
	h.supp.set(true)
	sender.down(ch, false)
	h.pass(time.Minute, 10)
	assert.Empty(t, sender.delivered(ch), "no retry goes out while a maintenance window covers the alert")

	h.supp.set(false)
	h.pass(time.Minute, 5)
	assert.Equal(t, 1, count(sender.delivered(ch), a.Message), "the page goes out once the window is over")
}
