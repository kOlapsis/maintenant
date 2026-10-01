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

// A pending update leaves only through a scan, which announces its recovery: the
// retention must neither delete it behind the alert's back nor, by purging its
// scan record, hide it from the next scan's staleness check.
func TestCleanupExpired_KeepsPendingUpdatesForTheNextScanToResolve(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	seedScannedContainer(t, cs, "ext-web", "web", nil)
	longAgo := time.Now().Add(-40 * 24 * time.Hour)
	oldScan, err := us.InsertScanRecord(ctx, &update.ScanRecord{StartedAt: longAgo, Status: update.ScanStatusCompleted})
	require.NoError(t, err)
	_, err = us.InsertImageUpdate(ctx, &update.ImageUpdate{
		ScanID: oldScan, ContainerID: "ext-web", ContainerName: "web", Image: "nginx",
		CurrentTag: "1.0.0", Registry: "docker.io", LatestTag: "2.0.0", UpdateType: update.UpdateTypeMajor,
		Status: update.StatusAvailable, DetectedAt: longAgo,
	})
	require.NoError(t, err)

	_, err = us.CleanupExpired(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)

	pending, err := us.GetImageUpdateByContainer(ctx, "ext-web")
	require.NoError(t, err)
	require.NotNil(t, pending, "an update still pending must survive the retention")

	newScan := seedScan(t, us)
	stale, err := us.ListStaleImageUpdates(ctx, newScan, []string{"web"})
	require.NoError(t, err)
	assert.Equal(t, []update.StaleImageUpdate{{ContainerID: "ext-web", ContainerName: "web"}}, stale)

	deleted, err := us.DeleteStaleImageUpdates(ctx, newScan, []string{"web"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
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
