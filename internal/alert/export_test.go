// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"context"
	"testing"
	"time"
)

// SetSilenceRecheckInterval sets how often the engine looks for held alerts to promote. Call it before Start.
func (e *Engine) SetSilenceRecheckInterval(d time.Duration) { e.recheckEvery = d }

// Settle returns once the event the engine is processing, if any, is fully applied to its maps.
func (e *Engine) Settle() {
	e.procMu.Lock()
	defer e.procMu.Unlock()
}

// SetClock sets the clock the notifier schedules and expires deliveries with. Call it before Start.
func (n *Notifier) SetClock(now func() time.Time) { n.now = now }

// SetResumeInterval sets how often the notifier looks for due deliveries. Call it before Start.
func (n *Notifier) SetResumeInterval(d time.Duration) { n.resumeEvery = d }

// StartUntilCleanup starts n, then stops it and waits for its workers when t ends.
func (n *Notifier) StartUntilCleanup(ctx context.Context, t testing.TB) {
	ctx, cancel := context.WithCancel(ctx)
	n.Start(ctx)
	t.Cleanup(func() {
		cancel()
		_ = n.Wait(context.Background())
	})
}

// QueuedAlerts drains the alert payloads waiting in a notifier that was never started.
func (n *Notifier) QueuedAlerts() []*Alert {
	var out []*Alert
	for _, q := range n.queues {
		for drained := false; !drained; {
			select {
			case job := <-q:
				out = append(out, job.Alert)
			default:
				drained = true
			}
		}
	}
	return out
}
