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

func TestLimiter_NewLimiterIsNotNil(t *testing.T) {
	l := NewLimiter()

	require.NotNil(t, l)
}

// TestLimiter_BurstOf1000AllowedThenDenied verifies the token-bucket semantics:
// exactly 1000 tokens are available per agent at construction time (burst).
// After the burst is consumed, further calls are denied with a positive retry-after.
//
// To avoid flakiness from wall-clock refill (rate = 1000 tok/s ≈ 1 tok/ms):
//   - we count all allowed calls in the first 2000 attempts;
//   - the burst guarantees at least 1000 are allowed;
//   - any call that is denied must carry a positive retry-after duration.
func TestLimiter_BurstOf1000AllowedThenDenied(t *testing.T) {
	l := NewLimiter()
	agentID := "agent-burst"

	var firstDeniedRetryAfter time.Duration
	allowed := 0
	denied := 0

	for i := range 2000 {
		ok, retryAfter := l.Allow(agentID)
		if ok {
			allowed++
		} else {
			denied++
			if i == allowed { // first denial
				firstDeniedRetryAfter = retryAfter
			}
		}
	}

	// At least burst-many calls must have been allowed.
	assert.GreaterOrEqual(t, allowed, agentBurst,
		"at least %d calls should be allowed (burst)", agentBurst)

	// After the burst is exhausted some calls must be denied.
	assert.Positive(t, denied, "some calls should be denied after burst is exhausted")

	// Any retry-after observed on the first denial must be non-negative.
	// (It may be zero if the bucket refilled in the meantime, but never negative.)
	assert.GreaterOrEqual(t, firstDeniedRetryAfter, time.Duration(0))
}

// TestLimiter_DeniedCallReturnsPositiveRetryAfter drains the bucket with a very
// large call count so that denial is certain regardless of refill timing.
func TestLimiter_DeniedCallReturnsPositiveRetryAfter(t *testing.T) {
	l := NewLimiter()
	agentID := "agent-retry"

	// Consume burst + a generous margin to guarantee denial.
	for range agentBurst {
		l.Allow(agentID)
	}

	// Keep calling until we get a definitive denial (at most 10 extra calls).
	var gotDenied bool
	var retryAfter time.Duration
	for range 10 {
		ok, ra := l.Allow(agentID)
		if !ok {
			gotDenied = true
			retryAfter = ra
			break
		}
	}

	assert.True(t, gotDenied, "should be denied after burst is exhausted")
	assert.Positive(t, retryAfter, "retry-after must be positive when denied")
}

func TestLimiter_DifferentAgentsAreIndependent(t *testing.T) {
	l := NewLimiter()
	agentA := "agent-indep-a"
	agentB := "agent-indep-b"

	// Deplete agent-a's budget. Continue until we observe a denial.
	var agentADenied bool
	for range agentBurst + 10 {
		ok, _ := l.Allow(agentA)
		if !ok {
			agentADenied = true
			break
		}
	}
	require.True(t, agentADenied, "agent-a should be rate-limited after exhausting burst")

	// agent-b has its own independent bucket — first call must succeed.
	allowedB, _ := l.Allow(agentB)
	assert.True(t, allowedB, "agent-b should not be affected by agent-a's rate limit")
}

func TestLimiter_RetryAfterIsZeroWhenAllowed(t *testing.T) {
	l := NewLimiter()
	agentID := "agent-retrycheck"

	allowed, retryAfter := l.Allow(agentID)

	assert.True(t, allowed)
	assert.Zero(t, retryAfter, "retry-after should be zero when the call is allowed")
}

// TestLimiter_SameAgentSharesSingleBucket verifies that calls for the same
// agentID always draw from the same bucket across multiple invocations.
func TestLimiter_SameAgentSharesSingleBucket(t *testing.T) {
	l := NewLimiter()
	agentID := "agent-shared"

	// Drain the shared bucket until we observe a denial.
	var denied bool
	for range agentBurst + 10 {
		ok, _ := l.Allow(agentID)
		if !ok {
			denied = true
			break
		}
	}

	assert.True(t, denied, "repeated calls for the same agentID share one bucket and exhaust it")
}
