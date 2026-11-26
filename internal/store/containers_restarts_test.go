// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountRestartsSince_CountsRestartsAfterACleanExit(t *testing.T) {
	db := openTestDB(t)
	cs := NewContainerStore(db)
	cid := seedHostContainer(t, cs, "ext-clean-loop", "")
	now := time.Now()
	for i, prev := range []string{"exited", "restarting", "completed"} {
		addTransition(t, db, cid, prev, "running", now.Add(time.Duration(i-3)*time.Minute))
	}
	addTransition(t, db, cid, "created", "running", now.Add(-time.Minute))

	n, err := cs.CountRestartsSince(context.Background(), cid, now.Add(-10*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}
