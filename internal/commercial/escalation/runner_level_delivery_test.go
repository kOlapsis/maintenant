// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
)

func TestLevelStaysDueWhileItsChannelCannotBeServed(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}},
		esc.Level{Order: 1, DelaySeconds: 600, ChannelIDs: []string{ch}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)

	h.channels.failGet(ch, 1)
	h.cycle()
	run := h.onlyRun(a)
	assert.Equal(t, -1, run.LastExecutedLevelIndex, "a channel that could not be read leaves the level due")
	assert.Empty(t, h.deliveries(run.ID))

	h.store.reserveFails.Store(1)
	h.cycle()
	assert.Equal(t, -1, h.onlyRun(a).LastExecutedLevelIndex, "a delivery that could not be reserved leaves the level due")
	assert.Empty(t, h.deliveries(run.ID))

	h.cycle()
	h.settle()
	assert.Equal(t, 0, h.onlyRun(a).LastExecutedLevelIndex, "the level goes out once the store answers")
	ds := h.deliveries(run.ID)
	require.Len(t, ds, 1)
	assert.Equal(t, esc.DeliveryStatusSent, ds[0].Status)

	h.cycle()
	h.settle()
	assert.Equal(t, 1, sentTo(sender.snapshot(), ch), "the level reaches its channel once")
}

func TestLevelRetryLeavesServedChannelsAlone(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	first := h.addChannel("first", "https://example.test/first", true)
	second := h.addChannel("second", "https://example.test/second", true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{first, second}},
		esc.Level{Order: 1, DelaySeconds: 600, ChannelIDs: []string{first}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)

	h.channels.failGet(second, 1)
	h.cycle()
	h.settle()
	assert.Equal(t, -1, h.onlyRun(a).LastExecutedLevelIndex, "the level waits for its second channel")

	h.cycle()
	h.settle()
	assert.Equal(t, 0, h.onlyRun(a).LastExecutedLevelIndex)
	calls := sender.snapshot()
	assert.Equal(t, 1, sentTo(calls, first), "the channel served by the first attempt is not notified again")
	assert.Equal(t, 1, sentTo(calls, second))
}

func TestLevelSkipsDisabledAndDeletedChannelsWithATrace(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	off := h.addChannel("off", "https://example.test/off", false)
	gone := h.addChannel("gone", "https://example.test/gone", true)
	a := h.addAlert()
	h.addPolicy(
		esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{off, gone}},
		esc.Level{Order: 1, DelaySeconds: 600, ChannelIDs: []string{off}},
	)
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	require.NoError(t, h.channels.DeleteChannel(h.ctx, gone))
	h.advance(61 * time.Second)

	h.cycle()
	h.settle()
	run := h.onlyRun(a)
	assert.Equal(t, 0, run.LastExecutedLevelIndex, "a channel set aside for good does not hold the level back")

	byChannel := map[string]*esc.Delivery{}
	for _, d := range h.deliveries(run.ID) {
		key := ""
		if d.ChannelID != nil {
			key = *d.ChannelID
		}
		byChannel[key] = d
	}
	require.Len(t, byChannel, 2, "each channel of the level leaves a trace")
	require.Contains(t, byChannel, off)
	assert.Equal(t, esc.DeliveryStatusFailed, byChannel[off].Status)
	assert.Equal(t, "channel disabled", byChannel[off].Error)
	require.Contains(t, byChannel, "", "the deleted channel is traced without its foreign key")
	assert.Equal(t, esc.DeliveryStatusFailed, byChannel[""].Status)
	assert.Contains(t, byChannel[""].Error, gone)
	assert.Empty(t, sender.snapshot())
}

func TestOrphanDeliveryWaitsOutAStorageError(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})
	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	run := h.onlyRun(a)
	_, err := h.store.Store.InsertDelivery(h.ctx, &esc.Delivery{
		RunID: run.ID, LevelIndex: 0, ChannelID: &ch,
		Status: esc.DeliveryStatusPending, AttemptStartedAt: h.clock().Add(-2 * time.Hour),
	})
	require.NoError(t, err)

	h.channels.failGet(ch, 1)
	h.cycle()
	ds := h.deliveries(run.ID)
	require.Len(t, ds, 1)
	assert.Equal(t, esc.DeliveryStatusPending, ds[0].Status, "a read error leaves the orphan for the next cycle")

	h.cycle()
	h.settle()
	ds = h.deliveries(run.ID)
	require.Len(t, ds, 1)
	assert.Equal(t, esc.DeliveryStatusSent, ds[0].Status)
	assert.Equal(t, 1, sentTo(sender.snapshot(), ch))
}
