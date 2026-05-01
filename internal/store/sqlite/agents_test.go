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

package sqlite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeAtomic_Concurrent(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	// Insert a token valid for 1 hour.
	tok := &agent.EnrollmentToken{
		TokenID:   "testtoken01",
		Token:     "mnt_enr_testtoken",
		CreatedBy: "test",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, store.InsertToken(ctx, tok))

	const goroutines = 10
	var (
		successes atomic.Int32
		alreadyConsumed atomic.Int32
		other     atomic.Int32
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", i)
			err := store.ConsumeAtomic(ctx, tok.Token, agentID)
			switch {
			case err == nil:
				successes.Add(1)
			case err == agent.ErrTokenAlreadyConsumed:
				alreadyConsumed.Add(1)
			default:
				other.Add(1)
			}
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load(), "exactly one goroutine should succeed")
	assert.Equal(t, int32(goroutines-1), alreadyConsumed.Load(), "all others should get ErrTokenAlreadyConsumed")
	assert.Equal(t, int32(0), other.Load(), "no unexpected errors")
}

func TestConsumeAtomic_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	err := store.ConsumeAtomic(ctx, "mnt_enr_doesnotexist", "agent-x")
	assert.ErrorIs(t, err, agent.ErrTokenNotFound)
}

func TestConsumeAtomic_Expired(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	tok := &agent.EnrollmentToken{
		TokenID:   "expiredtoken01",
		Token:     "mnt_enr_expired",
		CreatedBy: "test",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour), // already expired
	}
	require.NoError(t, store.InsertToken(ctx, tok))

	err := store.ConsumeAtomic(ctx, tok.Token, "agent-y")
	assert.ErrorIs(t, err, agent.ErrTokenExpired)
}

func TestAgentStore_InsertGet(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	a := &agent.Agent{
		AgentID:         "test-agent-id-1",
		PublicKey:       []byte("pubkeyplaceholder12345678901234"),
		Hostname:        "myhost",
		Label:           "My Host",
		OSArch:          "linux/amd64",
		AgentVersion:    "1.0.0",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       now,
	}
	require.NoError(t, store.Insert(ctx, a))

	got, err := store.Get(ctx, a.AgentID)
	require.NoError(t, err)
	assert.Equal(t, a.AgentID, got.AgentID)
	assert.Equal(t, a.Label, got.Label)
	assert.Equal(t, a.Hostname, got.Hostname)
	assert.Equal(t, a.Status, got.Status)
}
