// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

// Package retry provides a small exponential backoff helper used by
// reconnect / retry loops across the codebase (gRPC stream reconnect,
// Docker engine reconnect, runtime probe retry).
package retry

import (
	"context"
	"math/rand"
	"time"
)

// maxShift bounds the attempt counter used in Min << attempt to avoid
// int64 overflow when callers leave the loop running for a long time.
// 1s << 30 is already ~34 years, more than any sane Max.
const maxShift = 30

// Backoff implements exponential backoff with optional jitter.
// It is NOT safe for concurrent use by multiple goroutines.
type Backoff struct {
	// Min is the base interval used for attempt 0. Must be > 0.
	Min time.Duration
	// Max is the upper bound on the (un-jittered) delay.
	Max time.Duration
	// Jitter spreads the delay by ±Jitter (e.g. 0.25 means ±25%).
	// 0 disables jitter and produces deterministic doubling.
	Jitter float64

	attempt int
}

// New creates a Backoff. Pass jitter = 0 to disable randomization.
func New(min, max time.Duration, jitter float64) *Backoff {
	return &Backoff{Min: min, Max: max, Jitter: jitter}
}

// Reset clears the attempt counter so the next Next() call returns Min again.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt count (0 before the first Next call).
func (b *Backoff) Attempt() int {
	return b.attempt
}

// Next returns the next backoff delay and advances the attempt counter.
// The base delay doubles on each call up to Max, then jitter is applied.
func (b *Backoff) Next() time.Duration {
	a := b.attempt
	b.attempt++
	if a > maxShift {
		a = maxShift
	}
	raw := b.Min << uint(a)
	if raw <= 0 || raw > b.Max {
		raw = b.Max
	}
	if b.Jitter == 0 {
		return raw
	}
	factor := 1 - b.Jitter + rand.Float64()*(2*b.Jitter) //nolint:gosec // non-crypto jitter
	return time.Duration(float64(raw) * factor)
}

// Sleep waits for the next backoff delay or until ctx is cancelled.
// Returns ctx.Err() on cancellation, nil if the delay elapsed normally.
func (b *Backoff) Sleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(b.Next()):
		return nil
	}
}
