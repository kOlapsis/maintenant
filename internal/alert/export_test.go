// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import "time"

// SetSilenceRecheckInterval sets how often the engine looks for held alerts to promote. Call it before Start.
func (e *Engine) SetSilenceRecheckInterval(d time.Duration) { e.recheckEvery = d }

// Settle returns once the event the engine is processing, if any, is fully applied to its maps.
func (e *Engine) Settle() {
	e.procMu.Lock()
	defer e.procMu.Unlock()
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
