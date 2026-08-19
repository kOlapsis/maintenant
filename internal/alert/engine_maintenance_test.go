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

package alert_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/maintenance"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestEngineSuppressesAlertDuringMaintenanceWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, store.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	alertStore := store.NewAlertStore(db)
	channelStore := store.NewChannelStore(db)
	triggerStore := store.NewTriggerStore(db)
	silenceStore := store.NewSilenceStore(db)
	maintenanceStore := store.NewMaintenanceStore(db)

	suppressor := maintenance.NewSuppressor(maintenanceStore, logger)

	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: channelStore,
		TriggerStore: triggerStore,
		SilenceStore: silenceStore,
		Logger:       logger,
	})
	engine.SetMaintenanceSuppressor(suppressor)
	engine.Start(ctx)

	// Seed: maintenance window covering container:42
	now := time.Now()
	rawDB := db.ReadDB()

	componentID := uid.New()
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO status_components (id, composition_mode, match_all_type, display_name, display_order, visible, created_at, updated_at)
		 VALUES (?, 'explicit', NULL, 'Test', 0, 1, ?, ?)`,
		componentID, now.Unix(), now.Unix(),
	)
	require.NoError(t, err)

	windowID := uid.New()
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO maintenance_windows (id, title, description, starts_at, ends_at, active, created_at, updated_at)
		 VALUES (?, 'Test', '', ?, ?, 0, ?, ?)`,
		windowID, now.Add(-time.Hour).Unix(), now.Add(time.Hour).Unix(), now.Unix(), now.Unix(),
	)
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO maintenance_components (maintenance_id, component_id) VALUES (?, ?)`,
		windowID, componentID,
	)
	require.NoError(t, err)

	// container:42 — entity_id is now a TEXT UUID; this test uses '42' as the id.
	_, err = rawDB.ExecContext(ctx,
		`INSERT INTO status_component_monitors (component_id, monitor_type, monitor_id) VALUES (?, 'container', '42')`,
		componentID,
	)
	require.NoError(t, err)

	// Push event for container:42 (covered by maintenance window)
	engine.EventChannel() <- alert.Event{
		Source: "container", AlertType: "restart", Severity: "warning",
		EntityType: "container", EntityID: "42", EntityName: "c42", Timestamp: now,
	}
	time.Sleep(100 * time.Millisecond)

	// container:42 must NOT appear in active alerts
	activeAlerts, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	for _, a := range activeAlerts {
		if a.EntityType == "container" && a.EntityID == "42" {
			t.Fatal("container:42 should not have an active alert during maintenance window")
		}
	}

	// At least one silenced alert must be persisted for container:42 (audit trail)
	silenced, err := alertStore.ListAlerts(ctx, alert.ListAlertsOpts{Status: "silenced", Limit: 20})
	require.NoError(t, err)
	found := false
	for _, a := range silenced {
		if a.EntityType == "container" && a.EntityID == "42" {
			found = true
			assert.Equal(t, "silenced", a.Status)
		}
	}
	assert.True(t, found, "silenced audit record must exist for container:42")

	// Push event for container:99 (not covered) — must produce an active alert
	engine.EventChannel() <- alert.Event{
		Source: "container", AlertType: "restart", Severity: "warning",
		EntityType: "container", EntityID: "99", EntityName: "c99", Timestamp: now,
	}
	time.Sleep(100 * time.Millisecond)

	activeAlerts, err = alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	found99 := false
	for _, a := range activeAlerts {
		if a.EntityType == "container" && a.EntityID == "99" {
			found99 = true
			assert.Equal(t, "active", a.Status)
		}
	}
	assert.True(t, found99, "container:99 must have an active alert")
}
