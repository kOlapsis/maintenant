// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerStore_ArchivedPurgeCommitsBatchByBatch(t *testing.T) {
	prev := archivedPurgeBatch
	archivedPurgeBatch = 2
	t.Cleanup(func() { archivedPurgeBatch = prev })

	ctx := context.Background()
	db := openTestDB(t)
	if db.dialect == DialectPostgres {
		t.Skip("failure injection uses a SQLite trigger")
	}
	cs := NewContainerStore(db)
	seedAgent(t, db, "agent-ci")

	now := time.Now()
	ids := make([]string, 5)
	for i := range ids {
		ext := fmt.Sprintf("job-%d", i)
		ids[i] = seedHostedContainer(t, cs, "agent-ci", ext, ext, "job:1")
		require.NoError(t, cs.ArchiveContainer(ctx, ids[i], now.Add(-time.Duration(100-i)*time.Hour)))
	}
	kept := seedHostedContainer(t, cs, "agent-ci", "recent", "recent", "job:1")
	require.NoError(t, cs.ArchiveContainer(ctx, kept, now.Add(-time.Minute)))

	_, err := db.db.Exec(`CREATE TRIGGER fail_last BEFORE DELETE ON containers WHEN OLD.id = '` + ids[4] +
		`' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)

	deleted, err := cs.DeleteArchivedContainersBefore(ctx, now.Add(-time.Hour))
	require.Error(t, err)
	assert.Equal(t, int64(4), deleted, "the batches before the failing one are committed")
	remaining, err := cs.CountAgentContainers(ctx, "agent-ci")
	require.NoError(t, err)
	assert.Equal(t, 2, remaining)

	_, err = db.db.Exec(`DROP TRIGGER fail_last`)
	require.NoError(t, err)
	deleted, err = cs.DeleteArchivedContainersBefore(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	c, err := cs.GetContainerByID(ctx, kept)
	require.NoError(t, err)
	assert.NotNil(t, c, "a container archived after the cutoff stays")
}
