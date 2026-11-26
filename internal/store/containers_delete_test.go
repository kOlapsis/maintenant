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

func failDeletes(t *testing.T, db *DB, table string) {
	t.Helper()
	if db.dialect == DialectPostgres {
		t.Skip("failure injection uses a SQLite trigger")
	}
	_, err := db.db.Exec(`CREATE TRIGGER faild_` + table + ` BEFORE DELETE ON ` + table +
		` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
}

var containerPurges = []struct {
	name  string
	purge func(cs *ContainerStore, id string) error
}{
	{"delete", func(cs *ContainerStore, id string) error {
		return cs.DeleteContainerByID(context.Background(), id)
	}},
	{"archive_purge", func(cs *ContainerStore, id string) error {
		ctx := context.Background()
		if err := cs.ArchiveContainer(ctx, id, time.Now().Add(-48*time.Hour)); err != nil {
			return err
		}
		_, err := cs.DeleteArchivedContainersBefore(ctx, time.Now().Add(-time.Hour))
		return err
	}},
}

func TestContainerStore_PurgeKeepsEverythingWhenTheContainerDeleteFails(t *testing.T) {
	for _, tc := range containerPurges {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			stagingUID, _ := seedHomonyms(t, db)
			seedAgentRows(t, db, "agent-staging")
			failDeletes(t, db, "containers")

			require.Error(t, tc.purge(NewContainerStore(db), stagingUID))

			for _, table := range perAgentTables {
				assert.Equal(t, 1, agentRowCount(t, db, table, "agent-staging"), table)
			}
		})
	}
}

func TestContainerStore_PurgeRemovesEvaluationsAndAcknowledgments(t *testing.T) {
	for _, tc := range containerPurges {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			stagingUID, _ := seedHomonyms(t, db)
			seedAgentRows(t, db, "agent-staging")
			seedAgentRows(t, db, "agent-prod")

			require.NoError(t, tc.purge(NewContainerStore(db), stagingUID))

			for _, table := range []string{"cve_evaluations", "risk_acknowledgments"} {
				assert.Zero(t, agentRowCount(t, db, table, "agent-staging"), "%s of the purged workload", table)
				assert.Equal(t, 1, agentRowCount(t, db, table, "agent-prod"), "%s of the prod workload", table)
			}
		})
	}
}
