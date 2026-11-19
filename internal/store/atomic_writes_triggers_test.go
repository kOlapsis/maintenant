// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestTriggerStore_InsertRollsBackWhenChannelLinkFails(t *testing.T) {
	s, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()
	ch := seedChannelForTrigger(t, rawDB, "ch")

	_, err := s.InsertTrigger(ctx, &alert.AlertTrigger{Name: "dup", Enabled: true, ChannelIDs: []string{ch, ch}})
	require.Error(t, err)
	_, err = s.InsertTrigger(ctx, &alert.AlertTrigger{Name: "ghost", Enabled: true, ChannelIDs: []string{"missing"}})
	require.Error(t, err)

	var n int
	require.NoError(t, rawDB.QueryRow(`SELECT COUNT(*) FROM alert_triggers`).Scan(&n))
	assert.Zero(t, n, "a failed creation must leave no trigger behind")
}

func TestTriggerStore_UpdateRollsBackWhenChannelLinkFails(t *testing.T) {
	s, rawDB := setupTriggerTestDB(t)
	ctx := context.Background()
	c1 := seedChannelForTrigger(t, rawDB, "c1")
	c2 := seedChannelForTrigger(t, rawDB, "c2")

	id, err := s.InsertTrigger(ctx, &alert.AlertTrigger{Name: "before", Enabled: true, ChannelIDs: []string{c1}})
	require.NoError(t, err)

	for _, bad := range [][]string{{c2, c2}, {c2, "missing"}} {
		err = s.UpdateTrigger(ctx, &alert.AlertTrigger{ID: id, Name: "after", Enabled: true, ChannelIDs: bad})
		require.Error(t, err)

		got, err := s.GetTrigger(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "before", got.Name)
		assert.Equal(t, []string{c1}, got.ChannelIDs)
	}
}
