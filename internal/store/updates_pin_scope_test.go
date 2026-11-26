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

func TestAdoptVersionPin_OnlyForTheSameImageRepository(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs, us := NewContainerStore(db), NewUpdateStore(db)

	oldUID := seedHostedContainer(t, cs, uid.LocalAgent, "ctr-old", "db", "postgres:14.12")
	_, err := us.InsertVersionPin(ctx, &update.VersionPin{
		ContainerID: "ctr-old", ContainerName: "db", Image: "postgres:14.12", PinnedTag: "14.12", PinnedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, cs.ArchiveContainer(ctx, oldUID, time.Now()))

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-maria", "db", "mariadb:11.4.2")
	pin, err := us.AdoptVersionPin(ctx, "", "ctr-maria", "db", "mariadb:11.4.2")
	require.NoError(t, err)
	assert.Nil(t, pin, "a container of another image does not inherit the pin")

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-pg", "db", "docker.io/library/postgres:14.13@sha256:abc")
	pin, err = us.AdoptVersionPin(ctx, "", "ctr-pg", "db", "docker.io/library/postgres:14.13@sha256:abc")
	require.NoError(t, err)
	require.NotNil(t, pin, "the same repository under another tag is the same software")
	assert.Equal(t, "ctr-pg", pin.ContainerID)
}
