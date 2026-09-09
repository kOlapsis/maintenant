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

package ratelimit

import (
	"context"
	"sync"
	"time"
)

type visitor struct {
	mu        sync.Mutex
	tokens    float64
	lastRefil time.Time
}

// Limiter implements a per-IP token bucket rate limiter.
type Limiter struct {
	rate     float64
	burst    int
	resolver *ClientIPResolver
	visitors sync.Map // IP string → *visitor
}

// New creates a rate limiter allowing rate tokens per second with the given burst capacity.
func New(rate float64, burst int, resolver *ClientIPResolver) *Limiter {
	if resolver == nil {
		resolver = NewClientIPResolver(nil)
	}
	return &Limiter{
		rate:     rate,
		burst:    burst,
		resolver: resolver,
	}
}

// Allow consumes one token for the given IP and reports whether the request is allowed.
func (l *Limiter) Allow(ip string) bool {
	now := time.Now()

	val, loaded := l.visitors.LoadOrStore(ip, &visitor{
		tokens:    float64(l.burst) - 1,
		lastRefil: now,
	})
	if !loaded {
		return true
	}

	v := val.(*visitor)
	v.mu.Lock()
	defer v.mu.Unlock()

	elapsed := now.Sub(v.lastRefil).Seconds()
	v.tokens += elapsed * l.rate
	if v.tokens > float64(l.burst) {
		v.tokens = float64(l.burst)
	}
	v.lastRefil = now

	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

// Len reports how many addresses currently hold a bucket.
func (l *Limiter) Len() int {
	n := 0
	l.visitors.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// Start launches a background goroutine that evicts inactive visitors every 60 seconds.
// It stops when ctx is cancelled.
func (l *Limiter) Start(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	idle := l.idleBeforeEviction()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cutoff := now.Add(-idle)
			l.visitors.Range(func(key, val any) bool {
				v := val.(*visitor)
				v.mu.Lock()
				inactive := v.lastRefil.Before(cutoff)
				v.mu.Unlock()
				if inactive {
					l.visitors.Delete(key)
				}
				return true
			})
		}
	}
}

// idleBeforeEviction is how long a visitor must go untouched before its bucket is
// dropped. A bucket may only be forgotten once it has refilled to its burst: on a
// long window (a few tokens per hour) an earlier eviction would hand the quota back.
func (l *Limiter) idleBeforeEviction() time.Duration {
	const floor = 3 * time.Minute
	if l.rate <= 0 {
		return floor
	}
	refill := time.Duration(float64(l.burst) / l.rate * float64(time.Second))
	if refill < floor {
		return floor
	}
	return refill
}
