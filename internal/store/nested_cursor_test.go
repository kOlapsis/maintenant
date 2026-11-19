// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/status"
)

// Two connections are enough to deadlock a list that keeps its cursor open
// while it queries for each row: every concurrent caller holds one connection
// and waits for a second one.
func runConcurrentlyOnTwoConnections(t *testing.T, db *DB, calls ...func(ctx context.Context) error) {
	t.Helper()
	db.db.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = call(ctx)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "call %d", i)
	}
}

func TestListActiveIncidents_DoesNotHoldAConnectionPerRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewIncidentStore(db)
	c := seedComponent(t, db, "api")
	for _, title := range []string{"first", "second"} {
		_, err := s.CreateIncident(ctx, &status.Incident{Title: title, Severity: status.SeverityMinor, Status: status.IncidentInvestigating}, []string{c}, "details")
		require.NoError(t, err)
	}

	list := func(ctx context.Context) error {
		got, err := s.ListActiveIncidents(ctx)
		if err == nil {
			assert.Len(t, got, 2)
			for _, inc := range got {
				assert.Len(t, inc.Components, 1)
				assert.NotEmpty(t, inc.Updates)
			}
		}
		return err
	}
	first, err := s.ListActiveIncidents(ctx)
	require.NoError(t, err)
	write := func(ctx context.Context) error {
		_, err := s.CreateUpdate(ctx, &status.IncidentUpdate{IncidentID: first[0].ID, Status: status.IncidentIdentified, Message: "more"})
		return err
	}
	runConcurrentlyOnTwoConnections(t, db, list, list, list, list, list, list, write)
}

func TestMaintenanceWindowLists_DoNotHoldAConnectionPerRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewMaintenanceStore(db)
	c := seedComponent(t, db, "api")
	now := time.Now()
	for _, title := range []string{"first", "second"} {
		_, err := s.CreateMaintenance(ctx, &status.MaintenanceWindow{
			Title: title, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
		}, []string{c})
		require.NoError(t, err)
	}

	pending := func(ctx context.Context) error {
		got, err := s.GetPendingActivation(ctx, now.Unix())
		if err == nil {
			assert.Len(t, got, 2)
			for _, mw := range got {
				assert.Len(t, mw.Components, 1)
			}
		}
		return err
	}
	write := func(ctx context.Context) error {
		return s.SetActive(ctx, "none", true, nil)
	}
	runConcurrentlyOnTwoConnections(t, db, pending, pending, pending, pending, pending, pending, write)
}
