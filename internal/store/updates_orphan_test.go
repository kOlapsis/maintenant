// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/update"
)

// seedScannedContainer inserts a running container and, when archivedAt is set,
// archives it the way a destroy event or an agent inventory would.
func seedScannedContainer(t *testing.T, cs *ContainerStore, externalID, name string, archivedAt *time.Time) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	id, err := cs.InsertContainer(ctx, &container.Container{
		ExternalID:        externalID,
		Name:              name,
		Image:             "ghcr.io/acme/api:1.0.0",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	})
	require.NoError(t, err)

	if archivedAt != nil {
		require.NoError(t, cs.ArchiveContainer(ctx, externalID, *archivedAt))
	}
	return id
}

func seedImageUpdate(t *testing.T, us *UpdateStore, scanID, externalID, name string) {
	t.Helper()
	_, err := us.InsertImageUpdate(context.Background(), &update.ImageUpdate{
		ScanID:        scanID,
		ContainerID:   externalID,
		ContainerName: name,
		Image:         "ghcr.io/acme/api",
		CurrentTag:    "1.0.0",
		Registry:      "ghcr.io",
		LatestTag:     "2.0.0",
		UpdateType:    update.UpdateTypeMajor,
		RiskScore:     update.BaseRiskScore(update.UpdateTypeMajor),
		Status:        update.StatusAvailable,
		DetectedAt:    time.Now(),
	})
	require.NoError(t, err)
}

func seedScan(t *testing.T, us *UpdateStore) string {
	t.Helper()
	scanID, err := us.InsertScanRecord(context.Background(), &update.ScanRecord{
		StartedAt: time.Now(),
		Status:    update.ScanStatusCompleted,
	})
	require.NoError(t, err)
	return scanID
}

// Issue #50: `docker stack deploy` replaces a Swarm task with one under a new id
// and a new name (`stack_api.1.<task-id>`), so the finding attached to the old
// task is never matched by the name-based staleness check and stays on the
// Updates page for good.
func TestOrphanImageUpdates_SwarmTaskReplacedByDeploy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	archivedAt := time.Now().Add(-time.Hour)
	oldUID := seedScannedContainer(t, cs, "task-old", "stack_api.1.oldtask", &archivedAt)
	seedScannedContainer(t, cs, "task-new", "stack_api.1.newtask", nil)

	oldScan := seedScan(t, us)
	newScan := seedScan(t, us)
	seedImageUpdate(t, us, oldScan, "task-old", "stack_api.1.oldtask")
	seedImageUpdate(t, us, newScan, "task-new", "stack_api.1.newtask")

	// The name-based cleanup cannot see the replaced task: its name is gone from
	// the scan set, so it is neither listed nor deleted.
	stale, err := us.ListStaleImageUpdates(ctx, newScan, []string{"stack_api.1.newtask"})
	require.NoError(t, err)
	assert.Empty(t, stale, "the old task name is no longer scanned, so it never matches")

	// The Updates page must not serve the replaced task in the meantime.
	listed, err := us.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	require.NoError(t, err)
	require.Len(t, listed, 1, "only the running task has a finding to show")
	assert.Equal(t, "task-new", listed[0].ContainerID)

	orphans, err := us.ListOrphanImageUpdates(ctx)
	require.NoError(t, err)
	require.Len(t, orphans, 1)
	assert.Equal(t, "task-old", orphans[0].ContainerID)
	assert.Equal(t, "stack_api.1.oldtask", orphans[0].ContainerName)
	assert.Equal(t, oldUID, orphans[0].ContainerUID,
		"the uid must come along so the recovery event resolves the right alert")

	deleted, err := us.DeleteOrphanImageUpdates(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	remaining, err := us.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	require.NoError(t, err)
	require.Len(t, remaining, 1, "the running task keeps its finding")
	assert.Equal(t, "task-new", remaining[0].ContainerID)

	orphans, err = us.ListOrphanImageUpdates(ctx)
	require.NoError(t, err)
	assert.Empty(t, orphans)
}

// A finding whose container row was deleted outright (no archived row left to
// join) is still shed — with an empty uid, which the caller falls back on.
func TestOrphanImageUpdates_ContainerRowGone(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)

	seedImageUpdate(t, us, seedScan(t, us), "task-vanished", "stack_api.1.vanished")

	orphans, err := us.ListOrphanImageUpdates(ctx)
	require.NoError(t, err)
	require.Len(t, orphans, 1)
	assert.Equal(t, "task-vanished", orphans[0].ContainerID)
	assert.Empty(t, orphans[0].ContainerUID)

	deleted, err := us.DeleteOrphanImageUpdates(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}

// Archiving leaves the container state untouched, so a replaced Swarm task stays
// 'running' in the table: it must count neither as tracked nor as up to date.
func TestGetUpdateSummary_SkipsArchivedContainers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	archivedAt := time.Now().Add(-time.Hour)
	seedScannedContainer(t, cs, "task-old", "stack_api.1.oldtask", &archivedAt)
	seedScannedContainer(t, cs, "task-new", "stack_api.1.newtask", nil)

	scanID := seedScan(t, us)
	seedImageUpdate(t, us, scanID, "task-old", "stack_api.1.oldtask")
	seedImageUpdate(t, us, scanID, "task-new", "stack_api.1.newtask")

	summary, err := us.GetUpdateSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Critical, "only the running task has a pending major update")
	assert.Equal(t, 0, summary.UpToDate, "the archived task is not a container waiting to be scanned")
}

// The Updates page counts CVEs next to the image updates, so they must be shed
// with the task they belong to.
func TestListCVEs_SkipsContainersThatNoLongerExist(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	archivedAt := time.Now().Add(-time.Hour)
	seedScannedContainer(t, cs, "task-old", "stack_api.1.oldtask", &archivedAt)
	seedScannedContainer(t, cs, "task-new", "stack_api.1.newtask", nil)

	for _, externalID := range []string{"task-old", "task-new"} {
		require.NoError(t, us.UpsertContainerCVE(ctx, &update.ContainerCVE{
			ContainerID:     externalID,
			CVEID:           "CVE-2026-0001",
			Severity:        update.CVESeverityHigh,
			CVSSScore:       8.1,
			FirstDetectedAt: time.Now(),
		}))
	}

	cves, err := us.ListAllActiveCVEs(ctx, update.ListCVEsOpts{})
	require.NoError(t, err)
	require.Len(t, cves, 1)
	assert.Equal(t, "task-new", cves[0].ContainerID)

	counts, err := us.GetCVESummaryCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, counts["high"], "the replaced task must not be counted twice")
}

// Purging archived containers used to leave their update artifacts behind: the
// tables keyed by external_id have no FK, so nothing could ever reclaim them.
func TestDeleteArchivedContainersBefore_RemovesUpdateArtifacts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)

	longGone := time.Now().Add(-40 * 24 * time.Hour)
	seedScannedContainer(t, cs, "task-old", "stack_api.1.oldtask", &longGone)
	seedScannedContainer(t, cs, "task-new", "stack_api.1.newtask", nil)

	scanID := seedScan(t, us)
	for _, externalID := range []string{"task-old", "task-new"} {
		seedImageUpdate(t, us, scanID, externalID, "stack_api.1."+externalID)

		_, err := us.InsertVersionPin(ctx, &update.VersionPin{
			ContainerID: externalID,
			Image:       "ghcr.io/acme/api",
			PinnedTag:   "1.0.0",
			PinnedAt:    time.Now(),
		})
		require.NoError(t, err)

		require.NoError(t, us.UpsertContainerCVE(ctx, &update.ContainerCVE{
			ContainerID:     externalID,
			CVEID:           "CVE-2026-0001",
			Severity:        update.CVESeverityHigh,
			CVSSScore:       8.1,
			FirstDetectedAt: time.Now(),
		}))

		_, err = us.InsertRiskScoreRecord(ctx, &update.RiskScoreRecord{
			ContainerID: externalID,
			Score:       85,
			FactorsJSON: "[]",
			RecordedAt:  time.Now(),
		})
		require.NoError(t, err)
	}

	purged, err := cs.DeleteArchivedContainersBefore(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)

	for _, table := range []string{"image_updates", "container_cves", "version_pins", "risk_score_history"} {
		assert.Equal(t, 1, countRows(t, db.ReadDB(), table),
			"%s must keep the live container's row and only that one", table)
	}

	var leftover int
	require.NoError(t, db.ReadDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM image_updates WHERE container_id = 'task-old'`).Scan(&leftover))
	assert.Zero(t, leftover, "the purged container must not leave a finding behind")
}
