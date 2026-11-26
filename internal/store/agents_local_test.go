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
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestAgentStore_LocalSentinelCannotBeChanged(t *testing.T) {
	db := openTestDB(t)
	s := NewAgentStore(db)
	ctx := context.Background()

	now := time.Now().Unix()
	_, err := db.Writer().Exec(ctx,
		`INSERT INTO containers (id, external_id, name, image, state, first_seen_at, last_state_change_at, agent_id)
		 VALUES (?, 'ext', 'web', 'img', 'running', ?, ?, ?)`,
		uid.Container(uid.LocalAgent, "ext"), now, now, uid.LocalAgent)
	require.NoError(t, err)

	before, err := s.Get(ctx, uid.LocalAgent)
	require.NoError(t, err)

	assert.ErrorIs(t, s.Delete(ctx, uid.LocalAgent), agent.ErrAgentNotFound)
	assert.ErrorIs(t, s.UpdateLabel(ctx, uid.LocalAgent, "renamed"), agent.ErrAgentNotFound)
	assert.ErrorIs(t, s.Revoke(ctx, uid.LocalAgent, "admin"), agent.ErrAgentNotFound)

	after, err := s.Get(ctx, uid.LocalAgent)
	require.NoError(t, err, "the local row must survive")
	assert.Equal(t, before.Label, after.Label)
	assert.Equal(t, before.Status, after.Status)

	var n int
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM containers WHERE agent_id = ?`, uid.LocalAgent).Scan(&n))
	assert.Equal(t, 1, n, "local containers must survive")
}
