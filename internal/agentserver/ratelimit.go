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
	// defaultAgentEventsPerSecond is used when the configured rate is <= 0.
	defaultAgentEventsPerSecond = 1000
	limiterIdleTimeout          = time.Hour
)

type agentLimiter struct {
	limiter    *rate.Limiter
	lastUsedAt time.Time
}

// Limiter manages per-agent token-bucket rate limiters.
type Limiter struct {
	mu           sync.Mutex
	limiters     map[string]*agentLimiter
	eventsPerSec rate.Limit
	burst        int
}

// NewLimiter creates a per-agent Limiter allowing eventsPerSecond events per
// second for each agent, with an equal burst. A value <= 0 falls back to the
// default (1000/s). Driven by MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND.
func NewLimiter(eventsPerSecond int) *Limiter {
	if eventsPerSecond <= 0 {
		eventsPerSecond = defaultAgentEventsPerSecond
	}
	return &Limiter{
		limiters:     make(map[string]*agentLimiter),
		eventsPerSec: rate.Limit(eventsPerSecond),
		burst:        eventsPerSecond,
	}
}

// Allow reports whether agentID may process one more event right now.
// Returns (allowed, retryAfter). retryAfter is only meaningful when !allowed.
func (l *Limiter) Allow(agentID string) (bool, time.Duration) {
	l.mu.Lock()
	al, ok := l.limiters[agentID]
	if !ok {
		al = &agentLimiter{
			limiter: rate.NewLimiter(l.eventsPerSec, l.burst),
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
