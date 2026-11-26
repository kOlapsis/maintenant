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

// SetClock sets the clock the engine ages its pending notifications with. Call it before Start.
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

// SetMaxUnrouted sets how many notifications the engine keeps waiting for the store. Call it before Start.
func (e *Engine) SetMaxUnrouted(n int) { e.maxUnrouted = n }

// RetryStoreFailures runs the pass that the silence recheck runs after each promotion.
func (e *Engine) RetryStoreFailures(ctx context.Context) { e.retryStoreFailures(ctx) }

// Unrouted returns how many notifications wait for the store to be routed.
func (e *Engine) Unrouted() int {
	e.procMu.Lock()
	defer e.procMu.Unlock()
	return len(e.unrouted)
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
	for _, job := range n.drain() {
		out = append(out, job.Alert)
	}
	return out
}

// QueuedChannels drains the jobs waiting in a notifier that was never started and returns their channel ids.
func (n *Notifier) QueuedChannels() []string {
	var out []string
	for _, job := range n.drain() {
		out = append(out, job.Channel.ID)
	}
	return out
}

func (n *Notifier) drain() []NotificationJob {
	n.queuesMu.Lock()
	defer n.queuesMu.Unlock()
	var out []NotificationJob
	for _, q := range n.queues {
		for drained := false; !drained; {
			select {
			case job := <-q:
				out = append(out, job)
				delete(n.queued, job.Delivery.ID)
			default:
				drained = true
			}
		}
	}
	return out
}

// Resume runs one pass of the loop that hands the due deliveries to the workers.
func (n *Notifier) Resume(ctx context.Context) { n.resume(ctx) }

// SetWorkerIdle sets how long a channel's worker waits for a job before it stops. Call it before Start.
func (n *Notifier) SetWorkerIdle(d time.Duration) { n.workerIdle = d }

// ChannelQueues returns how many channel queues the notifier holds.
func (n *Notifier) ChannelQueues() int {
	n.queuesMu.Lock()
	defer n.queuesMu.Unlock()
	return len(n.queues)
}
