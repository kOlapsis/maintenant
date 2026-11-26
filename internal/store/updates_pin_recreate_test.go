// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestAdoptVersionPin_FollowsTheContainerThroughItsRecreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs, us := NewContainerStore(db), NewUpdateStore(db)

	oldUID := seedHostedContainer(t, cs, uid.LocalAgent, "ctr-old", "app-web-1", "nginx:1.25.3")
	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-other", "app-api-1", "nginx:1.25.3")
	_, err := us.InsertVersionPin(ctx, &update.VersionPin{
		ContainerID: "ctr-old", ContainerName: "app-web-1", Image: "nginx:1.25.3", PinnedTag: "1.25.3",
		Reason: "frozen", PinnedAt: time.Now(),
	})
	require.NoError(t, err)

	pin, err := us.AdoptVersionPin(ctx, "", "ctr-old", "app-web-1", "nginx:1.25.3")
	require.NoError(t, err)
	require.NotNil(t, pin)

	pin, err = us.AdoptVersionPin(ctx, "", "ctr-other", "app-api-1", "nginx:1.25.3")
	require.NoError(t, err)
	assert.Nil(t, pin, "another container is not pinned")

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-twin", "app-web-1", "nginx:1.25.3")
	pin, err = us.AdoptVersionPin(ctx, "", "ctr-twin", "app-web-1", "nginx:1.25.3")
	require.NoError(t, err)
	assert.Nil(t, pin, "a pin stays with its container for as long as that container lives")

	require.NoError(t, cs.ArchiveContainer(ctx, oldUID, time.Now()))
	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-new", "app-web-1", "nginx:1.25.3")

	pin, err = us.AdoptVersionPin(ctx, "", "ctr-new", "app-web-1", "nginx:1.25.3")
	require.NoError(t, err)
	require.NotNil(t, pin, "the recreated container keeps the pin")
	assert.Equal(t, "frozen", pin.Reason)

	_, err = cs.DeleteArchivedContainersBefore(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	pin, err = us.GetVersionPin(ctx, "", "ctr-new")
	require.NoError(t, err)
	require.NotNil(t, pin, "purging the former container leaves the pin of the new one")
	assert.Equal(t, 1, countRows(t, db.ReadDB(), "version_pins"))
}

func TestMigration51_PinsTakeTheNameOfTheirContainer(t *testing.T) {
	db := openTestSQLite(t)
	ctx := context.Background()
	cs, us := NewContainerStore(db), NewUpdateStore(db)
	head := m46Schema(t, db)

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-web", "app-web-1", "nginx:1.25.3")
	for _, id := range []string{"ctr-web", "ctr-gone"} {
		_, err := us.InsertVersionPin(ctx, &update.VersionPin{ContainerID: id, Image: "nginx:1.25.3", PinnedTag: "1.25.3", PinnedAt: time.Now()})
		require.NoError(t, err)
	}

	_, err := db.ReadDB().ExecContext(ctx, readMigration(t, "51", "down"))
	require.NoError(t, err)
	assert.NotContains(t, m46Schema(t, db).tables["version_pins"], "container_name")
	assert.Equal(t, 2, countRows(t, db.ReadDB(), "version_pins"), "the down migration keeps the pins")

	_, err = db.ReadDB().ExecContext(ctx, readMigration(t, "51", "up"))
	require.NoError(t, err)
	assert.Equal(t, head, m46Schema(t, db), "the migration rebuilds the head schema")

	var name string
	require.NoError(t, db.ReadDB().QueryRowContext(ctx,
		`SELECT container_name FROM version_pins WHERE container_id = 'ctr-web'`).Scan(&name))
	assert.Equal(t, "app-web-1", name)
	assert.Equal(t, 1, countRows(t, db.ReadDB(), "version_pins"), "a pin whose container is gone is dropped")
}
