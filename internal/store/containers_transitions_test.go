// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

func TestListTransitionsByContainer_IsCapped(t *testing.T) {
	const seeded = maxPageSize + 20
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	cid := seedHostContainer(t, cs, "ext-restart-loop", "")
	start := time.Now().Add(-time.Hour)
	for i := range seeded {
		_, err := cs.InsertTransition(ctx, &container.StateTransition{
			ContainerID: cid, PreviousState: container.StateExited, NewState: container.StateRunning,
			Timestamp: start.Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
	}

	for name, tc := range map[string]struct {
		limit, want int
	}{
		"no limit":        {0, 50},
		"explicit limit":  {10, 10},
		"oversized limit": {seeded, maxPageSize},
	} {
		t.Run(name, func(t *testing.T) {
			got, total, err := cs.ListTransitionsByContainer(ctx, cid, container.ListTransitionsOpts{Limit: tc.limit})
			require.NoError(t, err)
			assert.Equal(t, seeded, total)
			require.Len(t, got, tc.want)
			assert.Equal(t, start.Add((seeded-1)*time.Second).Unix(), got[0].Timestamp.Unix(), "newest first")
		})
	}
}
