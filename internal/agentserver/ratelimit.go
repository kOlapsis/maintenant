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

package agentserver

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	agentEventsPerSecond = 1000
	agentBurst           = 1000
	limiterIdleTimeout   = time.Hour
)

type agentLimiter struct {
	limiter    *rate.Limiter
	lastUsedAt time.Time
}

// Limiter manages per-agent token-bucket rate limiters.
type Limiter struct {
	mu       sync.Mutex
	limiters map[string]*agentLimiter
}

// NewLimiter creates a new per-agent Limiter.
func NewLimiter() *Limiter {
	return &Limiter{
		limiters: make(map[string]*agentLimiter),
	}
}

// Allow reports whether agentID may process one more event right now.
// Returns (allowed, retryAfter). retryAfter is only meaningful when !allowed.
func (l *Limiter) Allow(agentID string) (bool, time.Duration) {
	l.mu.Lock()
	al, ok := l.limiters[agentID]
	if !ok {
		al = &agentLimiter{
			limiter: rate.NewLimiter(agentEventsPerSecond, agentBurst),
		}
		l.limiters[agentID] = al
	}
	al.lastUsedAt = time.Now()
	l.mu.Unlock()

	if al.limiter.Allow() {
		return true, 0
	}
	r := al.limiter.Reserve()
	delay := r.Delay()
	r.Cancel()
	return false, delay
}

// Cleanup removes limiters that have been idle for more than limiterIdleTimeout.
func (l *Limiter) Cleanup() {
	cutoff := time.Now().Add(-limiterIdleTimeout)
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, al := range l.limiters {
		if al.lastUsedAt.Before(cutoff) {
			delete(l.limiters, id)
		}
	}
}
