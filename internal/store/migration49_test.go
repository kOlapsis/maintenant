// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

const m49Alert = "Endpoint http://10.0.3.7:8080/health?token=abc failed 3 consecutive checks"

func applyMigration49(t *testing.T, db *DB, direction string) {
	t.Helper()
	matches, err := fs.Glob(migrationFS, "migrations/"+db.dialect.String()+"/49_*."+direction+".sql")
	require.NoError(t, err)
	require.Len(t, matches, 1, "migration 49 (%s) must exist for %s", direction, db.dialect.String())
	sqlText, err := fs.ReadFile(migrationFS, matches[0])
	require.NoError(t, err)
	_, err = db.ReadDB().ExecContext(context.Background(), string(sqlText))
	require.NoError(t, err, direction)
}

type m49Incident struct {
	title   string
	updates []string
}

func m49Read(t *testing.T, incidents *IncidentStoreImpl, id string) m49Incident {
	t.Helper()
	inc, err := incidents.GetIncident(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, inc)
	out := m49Incident{title: inc.Title}
	for _, u := range inc.Updates {
		out.updates = append(out.updates, u.Message)
	}
	return out
}

func TestMigration49_RemovesAlertTextFromOldAutomaticIncidents(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	components := NewStatusComponentStore(db)
	incidents := NewIncidentStore(db)
	head := m46Schema(t, db)

	component := func(name string) string {
		id, err := components.CreateComponent(ctx, &status.Component{
			CompositionMode: status.CompositionMatchAll, MatchAllType: "endpoint", DisplayName: name, Visible: true,
		})
		require.NoError(t, err)
		return id
	}
	open := func(inc *status.Incident, message string, componentIDs ...string) string {
		inc.Severity = status.SeverityMajor
		inc.Status = status.IncidentInvestigating
		id, err := incidents.CreateIncident(ctx, inc, componentIDs, message)
		require.NoError(t, err)
		return id
	}
	autoUpdate := func(incidentID, incidentStatus, message string) {
		_, err := incidents.CreateUpdate(ctx, &status.IncidentUpdate{
			IncidentID: incidentID, Status: incidentStatus, Message: message, IsAuto: true,
		})
		require.NoError(t, err)
	}

	api, db2, web := component("API"), component("Database"), component("Web")

	applyMigration49(t, db, "down")

	oldAuto := open(&status.Incident{Title: "API - " + m49Alert}, m49Alert, api)
	autoUpdate(oldAuto, status.IncidentInvestigating, m49Alert)
	autoUpdate(oldAuto, status.IncidentResolved, "Auto-resolved: all monitors operational")

	oldAutoRenamed := open(&status.Incident{Title: "Old name - " + m49Alert}, m49Alert, api, db2)
	oldAutoOrphan := open(&status.Incident{Title: "Gone - " + m49Alert}, m49Alert)

	manual := open(&status.Incident{Title: "Payments are slow - we are on it"}, "Looking into the payment provider", web)
	autoUpdate(manual, status.IncidentInvestigating, m49Alert)

	newAuto := open(&status.Incident{Title: "Database - Major Outage"}, "Auto-detected: Major Outage", db2)
	autoUpdate(newAuto, status.IncidentInvestigating, "Auto-detected: Partial Outage")

	maintenance := open(&status.Incident{Title: "Upgrade - storage", IsMaintenance: true}, "storage", web)
	autoUpdate(maintenance, status.IncidentResolved, "Scheduled maintenance completed")

	applyMigration49(t, db, "up")
	assert.Equal(t, head, m46Schema(t, db), "the migration leaves the schema alone")

	want := map[string]m49Incident{
		oldAuto: {"API - Incident", []string{
			"Auto-detected", "Auto-detected", "Auto-resolved: all monitors operational"}},
		oldAutoRenamed: {"API - Incident", []string{"Auto-detected"}},
		oldAutoOrphan:  {"Incident", []string{"Auto-detected"}},
		manual: {"Payments are slow - we are on it", []string{
			"Looking into the payment provider", "Auto-detected"}},
		newAuto: {"Database - Major Outage", []string{
			"Auto-detected: Major Outage", "Auto-detected: Partial Outage"}},
		maintenance: {"Upgrade - storage", []string{"storage", "Scheduled maintenance completed"}},
	}
	for id, w := range want {
		got := m49Read(t, incidents, id)
		assert.Equal(t, w.title, got.title)
		assert.ElementsMatch(t, w.updates, got.updates, w.title)
		for _, text := range append([]string{got.title}, got.updates...) {
			assert.NotContains(t, text, "10.0.3.7")
			assert.NotContains(t, text, "token=abc")
		}
	}

	applyMigration49(t, db, "up")
	for id, w := range want {
		got := m49Read(t, incidents, id)
		assert.Equal(t, w.title, got.title, "a second run changes nothing")
		assert.ElementsMatch(t, w.updates, got.updates, "a second run changes nothing")
	}
}
