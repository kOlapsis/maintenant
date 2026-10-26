// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
)

// Acceptance test of 046-agent-grpc-hardening, FR-013: the read that lets
// RegisterAgent refuse an unknown token before opening a write transaction.
func TestTokenExists_ReportsStoredTokensWhateverTheirState(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	const fresh, consumed, expired = "mnt_enr_fresh000001", "mnt_enr_consumed0001", "mnt_enr_expired00001"
	require.NoError(t, store.InsertToken(ctx, tokenFor(fresh, now, now.Add(time.Hour))))
	require.NoError(t, store.InsertToken(ctx, tokenFor(consumed, now, now.Add(time.Hour))))
	require.NoError(t, store.InsertToken(ctx, tokenFor(expired, now.Add(-2*time.Hour), now.Add(-time.Hour))))
	require.NoError(t, store.EnrollAtomic(ctx, -1, consumed, enrollAgentRecord("a1b2c3d4-0000-4000-8000-000000000001")))

	for name, c := range map[string]struct {
		hash string
		want bool
	}{
		"fresh token":                 {agent.HashToken(fresh), true},
		"consumed token":              {agent.HashToken(consumed), true},
		"expired token":               {agent.HashToken(expired), true},
		"unknown token":               {agent.HashToken("mnt_enr_neverissued01"), false},
		"cleartext instead of a hash": {fresh, false},
		"empty hash":                  {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := store.TokenExists(ctx, c.hash)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
