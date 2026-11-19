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
