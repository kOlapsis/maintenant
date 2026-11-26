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

func TestContainerStore_DeleteOldestArchivedContainersOfOneAgent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	cs := NewContainerStore(db)
	stagingOldest, prod := seedHomonyms(t, db)
	seedAgentRows(t, db, "agent-staging")
	seedAgentRows(t, db, "agent-prod")

	now := time.Now()
	stagingOld := seedHostedContainer(t, cs, "agent-staging", "old", "old", "app:1")
	stagingRecent := seedHostedContainer(t, cs, "agent-staging", "recent", "recent", "app:1")
	stagingLive := seedHostedContainer(t, cs, "agent-staging", "live", "live", "app:1")
	require.NoError(t, cs.ArchiveContainer(ctx, stagingOldest, now.Add(-72*time.Hour)))
	require.NoError(t, cs.ArchiveContainer(ctx, stagingOld, now.Add(-48*time.Hour)))
	require.NoError(t, cs.ArchiveContainer(ctx, stagingRecent, now.Add(-time.Hour)))
	require.NoError(t, cs.ArchiveContainer(ctx, prod, now.Add(-96*time.Hour)))
	_, err := cs.InsertTransition(ctx, &container.StateTransition{
		ContainerID: stagingOld, PreviousState: container.StateRunning, NewState: container.StateExited, Timestamp: now,
	})
	require.NoError(t, err)

	stagingUndated := seedHostedContainer(t, cs, "agent-staging", "undated", "undated", "app:1")
	_, err = db.db.Exec(db.dialect.Rebind(`UPDATE containers SET archived=1, archived_at=NULL WHERE id=?`), stagingUndated)
	require.NoError(t, err)

	held, err := cs.CountAgentContainers(ctx, "agent-staging")
	require.NoError(t, err)
	assert.Equal(t, 5, held, "archived containers count, those of another agent do not")

	deleted, err := cs.DeleteOldestArchivedContainers(ctx, "agent-staging", 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	for id, kept := range map[string]bool{stagingOldest: false, stagingOld: false, stagingRecent: true, stagingLive: true, stagingUndated: true, prod: true} {
		c, err := cs.GetContainerByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, kept, c != nil, id)
	}
	_, total, err := cs.ListTransitionsByContainer(ctx, stagingOld, container.ListTransitionsOpts{Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "the rows that depend on a deleted container go with it")
	for _, table := range []string{"image_updates", "cve_evaluations", "risk_acknowledgments"} {
		assert.Zero(t, agentRowCount(t, db, table, "agent-staging"), "%s of the deleted container", table)
		assert.Equal(t, 1, agentRowCount(t, db, table, "agent-prod"), "%s of the other agent", table)
	}

	deleted, err = cs.DeleteOldestArchivedContainers(ctx, "agent-staging", 5)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "live containers are never deleted")
}
