// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
)

func (h *dbHarness) acknowledge(a *alert.Alert) {
	at := h.clock()
	acked, err := h.alerts.AcknowledgeAlert(h.ctx, a.ID, "alice", at)
	assert.NoError(h.t, err)
	assert.True(h.t, acked)
	assert.NoError(h.t, h.runner.OnAlertAcknowledged(h.ctx, a.ID, alert.Acknowledgment{By: "alice", At: at}))
}

func newTwoLevelRun(t *testing.T) (*dbHarness, *fakeSender, *alert.Alert, string) {
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
	return h, sender, a, ch
}

func TestAcknowledgmentRecordedWhileACycleReadsTheAlertStopsTheLevel(t *testing.T) {
	h, sender, a, _ := newTwoLevelRun(t)
	h.alerts.setHook(&h.alerts.afterGet, func() { h.acknowledge(a) })

	h.cycle()
	h.settle()
	run := h.onlyRun(a)
	assert.Equal(t, esc.RunStatusStoppedByAck, run.Status, "the cycle leaves the acknowledgment in place")
	assert.Empty(t, h.deliveries(run.ID), "no level goes out once the acknowledgment is recorded")

	h.advance(10 * time.Minute)
	h.cycle()
	h.settle()
	assert.Equal(t, esc.RunStatusStoppedByAck, h.onlyRun(a).Status)
	assert.Empty(t, sender.snapshot())
}

func TestAcknowledgmentRecordedWhileACycleWritesProgressKeepsTheRunStopped(t *testing.T) {
	h, sender, a, ch := newTwoLevelRun(t)
	h.alerts.setHook(&h.alerts.beforeEscalated, func() { h.acknowledge(a) })

	h.cycle()
	h.settle()
	assert.Equal(t, esc.RunStatusStoppedByAck, h.onlyRun(a).Status, "the progress write does not revive the run")
	assert.Equal(t, 2, sentTo(sender.snapshot(), ch), "the level reserved before the acknowledgment, then its notice")

	h.advance(10 * time.Minute)
	h.cycle()
	h.settle()
	assert.Equal(t, esc.RunStatusStoppedByAck, h.onlyRun(a).Status)
	assert.Equal(t, 2, sentTo(sender.snapshot(), ch), "no later level goes out")
}
