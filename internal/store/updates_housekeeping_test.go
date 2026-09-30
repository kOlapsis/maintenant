// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/update"
)

func TestCreateExclusion_ADuplicateReturnsTheStoredRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	first := &update.UpdateExclusion{Pattern: "nginx*", PatternType: update.ExclusionTypeImage, CreatedAt: time.Now().Add(-time.Hour)}
	created, err := us.CreateExclusion(ctx, first)
	require.NoError(t, err)
	assert.True(t, created)

	again := &update.UpdateExclusion{Pattern: "nginx*", PatternType: update.ExclusionTypeImage, CreatedAt: time.Now()}
	created, err = us.CreateExclusion(ctx, again)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, first.CreatedAt.Unix(), again.CreatedAt.Unix())

	other := &update.UpdateExclusion{Pattern: "nginx*", PatternType: update.ExclusionTypeTag, CreatedAt: time.Now()}
	created, err = us.CreateExclusion(ctx, other)
	require.NoError(t, err)
	assert.True(t, created, "the same pattern under another type is another exclusion")

	stored, err := us.ListExclusions(ctx)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	ids := []string{stored[0].ID, stored[1].ID}
	assert.ElementsMatch(t, []string{first.ID, other.ID}, ids)
}

func TestCleanupExpired_PurgesTheDigestBaselinesOfGoneContainers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	archived := time.Now().Add(-time.Hour)
	seedScannedContainer(t, cs, "ext-live", "live", nil)
	seedScannedContainer(t, cs, "ext-archived", "archived", &archived)

	for _, id := range []string{"ext-live", "ext-archived", "ext-never-seen"} {
		require.NoError(t, us.UpsertDigestBaseline(ctx, &update.DigestBaseline{
			ContainerID: id, Image: "nginx:latest", Tag: "latest", RemoteDigest: "sha256:abc", CheckedAt: time.Now(),
		}))
	}

	_, err := us.CleanupExpired(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)

	live, err := us.GetDigestBaseline(ctx, "ext-live")
	require.NoError(t, err)
	assert.NotNil(t, live, "a running container keeps its baseline")
	for _, id := range []string{"ext-archived", "ext-never-seen"} {
		b, err := us.GetDigestBaseline(ctx, id)
		require.NoError(t, err)
		assert.Nil(t, b, "the baseline of %s must be purged", id)
	}
}
