// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
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

	size        atomic.Int64
	maxVisitors int64
	lastSweep   atomic.Int64
}

const defaultMaxVisitors = 100_000

// New creates a rate limiter allowing rate tokens per second with the given burst capacity.
func New(rate float64, burst int, resolver *ClientIPResolver) *Limiter {
	if resolver == nil {
		resolver = NewClientIPResolver(nil)
	}
	return &Limiter{
		rate:     rate,
		burst:    burst,
		resolver: resolver,

		maxVisitors: defaultMaxVisitors,
	}
}

// Allow consumes one token for the given key and reports whether the request is allowed.
// Once the table is full of non-idle keys, a key it has never seen is refused.
func (l *Limiter) Allow(ip string) bool {
	now := time.Now()

	if _, known := l.visitors.Load(ip); !known && l.size.Load() >= l.maxVisitors && !l.makeRoom(now) {
		return false
	}

	val, loaded := l.visitors.LoadOrStore(ip, &visitor{
		tokens:    float64(l.burst) - 1,
		lastRefil: now,
	})
	if !loaded {
		l.size.Add(1)
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
			l.evictIdle(now.Add(-idle))
		}
	}
}

func (l *Limiter) evictIdle(cutoff time.Time) {
	l.visitors.Range(func(key, val any) bool {
		v := val.(*visitor)
		v.mu.Lock()
		inactive := v.lastRefil.Before(cutoff)
		v.mu.Unlock()
		if inactive {
			if _, deleted := l.visitors.LoadAndDelete(key); deleted {
				l.size.Add(-1)
			}
		}
		return true
	})
}

// makeRoom sweeps idle visitors, at most once a second so a flood of new keys cannot turn every refusal into a full scan.
func (l *Limiter) makeRoom(now time.Time) bool {
	last := l.lastSweep.Load()
	if now.UnixNano()-last >= int64(time.Second) && l.lastSweep.CompareAndSwap(last, now.UnixNano()) {
		l.evictIdle(now.Add(-l.idleBeforeEviction()))
	}
	return l.size.Load() < l.maxVisitors
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
