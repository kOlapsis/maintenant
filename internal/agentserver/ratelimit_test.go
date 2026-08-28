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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowRate keeps the exhaustion tests free of wall-clock refill: at 5 events per
// second the bucket regains a token every 200 ms, far more than a handful of
// calls takes, so draining the burst always ends on a denial. At the default
// 1000/s a token comes back every millisecond, fast enough to outrun the drain
// on a loaded machine.
const slowRate = 5

func TestLimiter_NewLimiterIsNotNil(t *testing.T) {
	l := NewLimiter(defaultAgentEventsPerSecond)

	require.NotNil(t, l)
}

// TestLimiter_DefaultBurstAllowsOneThousand verifies the token-bucket semantics
// of the default rate: 1000 tokens are available per agent at construction time,
// so the first 1000 calls all pass. Tokens only ever refill, never expire, which
// makes the count exact whatever the timing.
func TestLimiter_DefaultBurstAllowsOneThousand(t *testing.T) {
	l := NewLimiter(defaultAgentEventsPerSecond)
	agentID := "agent-burst"

	allowed := 0
	for range defaultAgentEventsPerSecond {
		if ok, _ := l.Allow(agentID); ok {
			allowed++
		}
	}

	assert.Equal(t, defaultAgentEventsPerSecond, allowed,
		"the first %d calls must all be allowed (burst)", defaultAgentEventsPerSecond)
}

// TestLimiter_DeniedCallReturnsPositiveRetryAfter drains the burst and checks the
// denial carries the delay until the next token.
func TestLimiter_DeniedCallReturnsPositiveRetryAfter(t *testing.T) {
	l := NewLimiter(slowRate)
	agentID := "agent-retry"

	for range slowRate {
		ok, _ := l.Allow(agentID)
		require.True(t, ok, "the burst must cover the first %d calls", slowRate)
	}

	ok, retryAfter := l.Allow(agentID)

	assert.False(t, ok, "should be denied after the burst is exhausted")
	assert.Positive(t, retryAfter, "retry-after must be positive when denied")
	assert.LessOrEqual(t, retryAfter, time.Second, "retry-after must not exceed the refill period")
}

func TestLimiter_DifferentAgentsAreIndependent(t *testing.T) {
	l := NewLimiter(slowRate)
	agentA := "agent-indep-a"
	agentB := "agent-indep-b"

	// Deplete agent-a's budget.
	for range slowRate {
		ok, _ := l.Allow(agentA)
		require.True(t, ok, "the burst must cover the first %d calls", slowRate)
	}
	allowedA, _ := l.Allow(agentA)
	require.False(t, allowedA, "agent-a should be rate-limited after exhausting its burst")

	// agent-b has its own independent bucket: the first call must succeed.
	allowedB, _ := l.Allow(agentB)
	assert.True(t, allowedB, "agent-b should not be affected by agent-a's rate limit")
}

func TestLimiter_RetryAfterIsZeroWhenAllowed(t *testing.T) {
	l := NewLimiter(defaultAgentEventsPerSecond)
	agentID := "agent-retrycheck"

	allowed, retryAfter := l.Allow(agentID)

	assert.True(t, allowed)
	assert.Zero(t, retryAfter, "retry-after should be zero when the call is allowed")
}

// TestLimiter_SameAgentSharesSingleBucket verifies that calls for the same
// agentID always draw from the same bucket: a later call sees the tokens the
// earlier ones consumed, and the agent holds a single entry.
func TestLimiter_SameAgentSharesSingleBucket(t *testing.T) {
	l := NewLimiter(slowRate)
	agentID := "agent-shared"

	for range slowRate {
		ok, _ := l.Allow(agentID)
		require.True(t, ok, "the burst must cover the first %d calls", slowRate)
	}

	ok, _ := l.Allow(agentID)
	assert.False(t, ok, "a later call must see the tokens consumed by the earlier ones")

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.Len(t, l.limiters, 1, "one bucket per agentID, not one per call")
}

// TestLimiter_RespectsConfiguredRate verifies the per-second rate passed to
// NewLimiter is honoured (burst equals the configured value), so
// MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND actually drives the limiter.
func TestLimiter_RespectsConfiguredRate(t *testing.T) {
	l := NewLimiter(5)
	agentID := "agent-configured"

	allowed := 0
	for range 5 {
		if ok, _ := l.Allow(agentID); ok {
			allowed++
		}
	}
	assert.Equal(t, 5, allowed, "burst should equal the configured rate (5)")

	// The next call (issued immediately, before any refill) exhausts the burst.
	ok, retryAfter := l.Allow(agentID)
	assert.False(t, ok, "6th call must be denied with rate/burst=5")
	assert.Positive(t, retryAfter, "denied call must carry a positive retry-after")
}

// TestLimiter_NonPositiveRateFallsBackToDefault verifies that a zero/negative
// configured rate falls back to the default rather than denying all events.
func TestLimiter_NonPositiveRateFallsBackToDefault(t *testing.T) {
	l := NewLimiter(0)

	allowed, _ := l.Allow("agent-default")

	assert.True(t, allowed, "a zero/negative configured rate must fall back to the default, not deny everything")
}
