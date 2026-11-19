// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/status"
)

func countAtomicRows(t *testing.T, db *DB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.db.QueryRow(query, args...).Scan(&n))
	return n
}

// failInserts makes every later INSERT on table raise, whatever its values.
func failInserts(t *testing.T, db *DB, table string) {
	t.Helper()
	if db.dialect == DialectPostgres {
		t.Skip("failure injection uses a SQLite trigger")
	}
	_, err := db.db.Exec(`CREATE TRIGGER fail_` + table + ` BEFORE INSERT ON ` + table +
		` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
}

func failUpdates(t *testing.T, db *DB, table string) {
	t.Helper()
	if db.dialect == DialectPostgres {
		t.Skip("failure injection uses a SQLite trigger")
	}
	_, err := db.db.Exec(`CREATE TRIGGER failu_` + table + ` BEFORE UPDATE ON ` + table +
		` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
}

func seedComponent(t *testing.T, db *DB, name string) string {
	t.Helper()
	id, err := NewStatusComponentStore(db).CreateComponent(context.Background(), &status.Component{
		CompositionMode: status.CompositionExplicit, DisplayName: name, Visible: true,
	})
	require.NoError(t, err)
	return id
}

func componentIDs(refs []status.IncidentCompRef) []string {
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return ids
}

func TestIncidentStore_CreateRollsBackWhenALinkFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewIncidentStore(db)
	c := seedComponent(t, db, "api")

	_, err := s.CreateIncident(ctx, &status.Incident{Title: "Down", Severity: status.SeverityMinor, Status: status.IncidentInvestigating},
		[]string{c, "missing"}, "first message")
	require.Error(t, err)
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM incidents`))
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM incident_updates`))
}

func TestIncidentStore_CreateRollsBackWhenTheInitialMessageFails(t *testing.T) {
	db := openTestDB(t)
	failInserts(t, db, "incident_updates")
	c := seedComponent(t, db, "api")

	_, err := NewIncidentStore(db).CreateIncident(context.Background(),
		&status.Incident{Title: "Down", Severity: status.SeverityMinor, Status: status.IncidentInvestigating}, []string{c}, "msg")
	require.Error(t, err)
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM incidents`))
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM incident_components`))
}

func TestIncidentStore_UpdateRollsBackWhenALinkFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewIncidentStore(db)
	c1 := seedComponent(t, db, "api")
	c2 := seedComponent(t, db, "web")
	id, err := s.CreateIncident(ctx, &status.Incident{Title: "before", Severity: status.SeverityMinor, Status: status.IncidentInvestigating}, []string{c1}, "")
	require.NoError(t, err)

	err = s.UpdateIncident(ctx, &status.Incident{ID: id, Title: "after", Severity: status.SeverityMajor}, []string{c2, "missing"})
	require.Error(t, err)

	got, err := s.GetIncident(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "before", got.Title)
	assert.Equal(t, []string{c1}, componentIDs(got.Components))
}

func TestIncidentStore_CreateUpdateRollsBackWhenTheStatusChangeFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewIncidentStore(db)
	id, err := s.CreateIncident(ctx, &status.Incident{Title: "Down", Severity: status.SeverityMinor, Status: status.IncidentInvestigating}, nil, "")
	require.NoError(t, err)
	failUpdates(t, db, "incidents")

	_, err = s.CreateUpdate(ctx, &status.IncidentUpdate{IncidentID: id, Status: status.IncidentResolved, Message: "fixed"})
	require.Error(t, err)
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM incident_updates`))
}

func TestMaintenanceStore_CreateRollsBackWhenALinkFails(t *testing.T) {
	db := openTestDB(t)
	c := seedComponent(t, db, "api")
	now := time.Now()

	_, err := NewMaintenanceStore(db).CreateMaintenance(context.Background(),
		&status.MaintenanceWindow{Title: "Upgrade", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)},
		[]string{c, "missing"})
	require.Error(t, err)
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM maintenance_windows`))
}

func TestMaintenanceStore_UpdateRollsBackWhenALinkFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewMaintenanceStore(db)
	c1 := seedComponent(t, db, "api")
	c2 := seedComponent(t, db, "web")
	now := time.Now()
	id, err := s.CreateMaintenance(ctx, &status.MaintenanceWindow{Title: "before", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)}, []string{c1})
	require.NoError(t, err)

	err = s.UpdateMaintenance(ctx, &status.MaintenanceWindow{ID: id, Title: "after", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)}, []string{c2, "missing"})
	require.Error(t, err)

	got, err := s.GetMaintenance(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "before", got.Title)
	assert.Equal(t, []string{c1}, componentIDs(got.Components))
}

func TestStatusComponentStore_CreateRollsBackWhenAMonitorRefFails(t *testing.T) {
	db := openTestDB(t)
	_, err := NewStatusComponentStore(db).CreateComponent(context.Background(), &status.Component{
		CompositionMode: status.CompositionExplicit, DisplayName: "api",
		Monitors: []status.MonitorRef{{Type: "endpoint", ID: "e1"}, {Type: "endpoint", ID: "e1"}},
	})
	require.Error(t, err)
	assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM status_components`))
}

func TestStatusComponentStore_UpdateRollsBackWhenAMonitorRefFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewStatusComponentStore(db)
	id, err := s.CreateComponent(ctx, &status.Component{
		CompositionMode: status.CompositionExplicit, DisplayName: "before",
		Monitors: []status.MonitorRef{{Type: "endpoint", ID: "e1"}},
	})
	require.NoError(t, err)
	failInserts(t, db, "status_component_monitors")

	err = s.UpdateComponent(ctx, &status.Component{
		ID: id, CompositionMode: status.CompositionExplicit, DisplayName: "after",
		Monitors: []status.MonitorRef{{Type: "endpoint", ID: "e2"}},
	})
	require.Error(t, err)

	got, err := s.GetComponent(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "before", got.DisplayName)
	require.Len(t, got.Monitors, 1)
	assert.Equal(t, "e1", got.Monitors[0].ID)
}

func TestKubernetesStore_ReplaceNamespacesKeepsThePreviousListOnFailure(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewKubernetesStore(db)
	seedTestAgent(t, db, "kube-A", "kubernetes")
	require.NoError(t, s.ReplaceNamespacesForAgent(ctx, "kube-A", []string{"default", "prod"}))

	require.Error(t, s.ReplaceNamespacesForAgent(ctx, "kube-A", []string{"a", "b", "b"}))

	got, err := s.ListNamespaces(ctx, "kube-A")
	require.NoError(t, err)
	assert.Equal(t, []string{"default", "prod"}, got)
}

func TestKubernetesStore_ReplaceEventsKeepsThePreviousListOnFailure(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewKubernetesStore(db)
	seedTestAgent(t, db, "kube-A", "kubernetes")
	ev := func(reason string) kubernetes.K8sEventRef {
		return kubernetes.K8sEventRef{
			K8sEvent:     kubernetes.K8sEvent{Reason: reason},
			InvolvedKind: "Pod", InvolvedNamespace: "default", InvolvedName: "web",
		}
	}
	require.NoError(t, s.ReplaceEventsForAgent(ctx, "kube-A", []kubernetes.K8sEventRef{ev("old")}))
	failInserts(t, db, "kubernetes_events")

	require.Error(t, s.ReplaceEventsForAgent(ctx, "kube-A", []kubernetes.K8sEventRef{ev("new")}))

	got, err := s.ListEventsForObject(ctx, "kube-A", "Pod", "default", "web")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "old", got[0].Reason)
}
