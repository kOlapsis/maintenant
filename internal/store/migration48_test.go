// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

func applyMigration48(t *testing.T, db *DB, direction string) {
	t.Helper()
	matches, err := fs.Glob(migrationFS, "migrations/"+db.dialect.String()+"/48_*."+direction+".sql")
	require.NoError(t, err)
	require.Len(t, matches, 1, "migration 48 (%s) must exist for %s", direction, db.dialect.String())
	sqlText, err := fs.ReadFile(migrationFS, matches[0])
	require.NoError(t, err)
	_, err = db.ReadDB().ExecContext(context.Background(), string(sqlText))
	require.NoError(t, err, direction)
}

func seedM48Alert(t *testing.T, as *AlertStoreImpl, source, alertType, severity, status, entityID string) string {
	t.Helper()
	id, err := as.InsertAlert(context.Background(), &alert.Alert{
		Source: source, AlertType: alertType, Severity: severity, Status: status,
		Message: "m", EntityType: "container", EntityID: entityID, EntityName: entityID, FiredAt: time.Now(),
	})
	require.NoError(t, err)
	return id
}

func seedM48Update(t *testing.T, us *UpdateStore, scanID, majorTag string) {
	t.Helper()
	_, err := us.InsertImageUpdate(context.Background(), &update.ImageUpdate{
		ScanID: scanID, ContainerID: "ctr-pg", ContainerName: "pg", Image: "postgres:18.7", CurrentTag: "18.7",
		Registry: "registry-1.docker.io", LatestTag: "18.8", MajorTag: majorTag, UpdateType: update.UpdateTypeMinor,
		RiskScore: update.BaseRiskScore(update.UpdateTypeMinor), Status: update.StatusAvailable, DetectedAt: time.Now(),
	})
	require.NoError(t, err)
}

// The down migration drops the column on a database that holds updates; the up
// migration adds it back and lowers the critical update alerts raised under the old rule.
func TestMigration48_MajorTagAndCriticalUpdateAlerts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs := NewContainerStore(db)
	us := NewUpdateStore(db)
	as := NewAlertStore(db)
	head := m46Schema(t, db)

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-pg", "pg", "postgres:18.7")
	scanID := seedScan(t, us)
	seedM48Update(t, us, scanID, "19.0")

	applyMigration48(t, db, "down")
	assert.NotContains(t, m46Schema(t, db).tables["image_updates"], "major_tag")
	var kept int
	require.NoError(t, db.ReadDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM image_updates`).Scan(&kept))
	assert.Equal(t, 1, kept, "the down migration keeps the updates")

	raised := seedM48Alert(t, as, "update", "update_available", alert.SeverityCritical, alert.StatusActive, "uid-pg")
	warning := seedM48Alert(t, as, "update", "update_available", alert.SeverityWarning, alert.StatusActive, "uid-web")
	silenced := seedM48Alert(t, as, "update", "update_available", alert.SeverityCritical, alert.StatusSilenced, "uid-quiet")
	resolved := seedM48Alert(t, as, "update", "update_available", alert.SeverityCritical, alert.StatusResolved, "uid-old")
	other := seedM48Alert(t, as, "container", "restart", alert.SeverityCritical, alert.StatusActive, "uid-pg")

	applyMigration48(t, db, "up")
	assert.Equal(t, head, m46Schema(t, db), "the migration rebuilds the head schema")

	for id, want := range map[string]string{
		raised:   alert.SeverityInfo,
		silenced: alert.SeverityInfo,
		warning:  alert.SeverityWarning,
		resolved: alert.SeverityCritical,
		other:    alert.SeverityCritical,
	} {
		a, err := as.GetAlert(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, a)
		assert.Equal(t, want, a.Severity, "%s %s %s", a.Source, a.Status, a.EntityID)
	}

	got, err := us.GetAgentImageUpdate(ctx, uid.LocalAgent, "ctr-pg")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.MajorTag, "the column comes back empty")
}

func TestImageUpdates_MajorTagIsStoredAndOverwritten(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	us := NewUpdateStore(db)
	seedHostedContainer(t, NewContainerStore(db), uid.LocalAgent, "ctr-pg", "pg", "postgres:18.7")
	scanID := seedScan(t, us)

	seedM48Update(t, us, scanID, "19.0")
	got, err := us.GetAgentImageUpdate(ctx, uid.LocalAgent, "ctr-pg")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "19.0", got.MajorTag)

	seedM48Update(t, us, scanID, "")
	listed, err := us.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Empty(t, listed[0].MajorTag, "the upsert overwrites the major tag")

	listed[0].MajorTag = "20.0"
	require.NoError(t, us.UpdateImageUpdate(ctx, listed[0]))
	got, err = us.GetImageUpdate(ctx, listed[0].ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "20.0", got.MajorTag)
}
