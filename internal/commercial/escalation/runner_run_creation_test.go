// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	esc "github.com/kolapsis/maintenant/internal/alert/escalation"
)

func TestRunCreationIsRetriedAfterAStorageError(t *testing.T) {
	h := newDBHarness(t, newFakeSender())
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{ch}})
	firedAt := h.clock()

	h.store.insertRunFails.Store(1)
	assert.Error(t, h.runner.OnAlertCreated(h.ctx, a), "the engine hears that the run could not start")
	assert.Empty(t, h.runs(a))

	h.advance(30 * time.Second)
	h.cycle()
	run := h.onlyRun(a)
	require.NotNil(t, run.NextActionAt)
	assert.True(t, firedAt.Add(60*time.Second).Equal(*run.NextActionAt), "the late run keeps the schedule of the alert, got %s", run.NextActionAt)

	require.NoError(t, h.runner.OnAlertCreated(h.ctx, a))
	h.cycle()
	assert.Len(t, h.runs(a), 1, "one run per alert and policy")
}

func TestRetriedRunCreationLeavesAFinishedRunAlone(t *testing.T) {
	sender := newFakeSender()
	h := newDBHarness(t, sender)
	first := h.addChannel("first", "https://example.test/first", true)
	second := h.addChannel("second", "https://example.test/second", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{first}})
	late := h.addPolicy(esc.Level{Order: 0, DelaySeconds: 60, ChannelIDs: []string{second}})

	h.store.refusedPolicyID.Store(&late)
	require.Error(t, h.runner.OnAlertCreated(h.ctx, a))
	h.advance(61 * time.Second)
	h.cycle()
	h.settle()
	h.cycle()
	h.settle()
	runs := h.runs(a)
	require.Len(t, runs, 1)
	require.Equal(t, esc.RunStatusExhausted, runs[0].Status)

	h.store.refusedPolicyID.Store(nil)
	h.cycle()
	h.settle()
	assert.Len(t, h.runs(a), 2, "the late policy starts, the exhausted one does not start again")
	calls := sender.snapshot()
	assert.Equal(t, 2, sentTo(calls, first), "its level and its exhaustion notice, once each")
	assert.Equal(t, 1, sentTo(calls, second))
}

func TestRunCreationRetryAndEngineCallMakeOneRun(t *testing.T) {
	h := newDBHarness(t, newFakeSender())
	ch := h.addChannel("ops", "https://example.test/ops", true)
	a := h.addAlert()
	h.addPolicy(esc.Level{Order: 0, DelaySeconds: 600, ChannelIDs: []string{ch}})

	h.store.insertRunFails.Store(1)
	require.Error(t, h.runner.OnAlertCreated(h.ctx, a))

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- h.runner.OnAlertCreated(h.ctx, a)
		}()
		go func() {
			defer wg.Done()
			errs <- h.runner.EvaluateCycle(h.ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}
	assert.Len(t, h.runs(a), 1, "the engine and the retry never both start the run")
}
