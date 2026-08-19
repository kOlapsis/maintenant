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

package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/store"
)

// TestPruneOrphanAlerts_ResolvesDeletedAgent verifies that a disconnect alert
// for an agent that no longer exists is resolved on startup, while a disconnect
// alert for an agent that still exists is left untouched.
func TestPruneOrphanAlerts_ResolvesDeletedAgent(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, store.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	alertStore := store.NewAlertStore(db)
	agentStore := store.NewAgentStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})

	// One enrolled agent still exists; the other was deleted.
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: "live-agent", Hostname: "live", Status: "active",
		DetectedRuntime: "docker", CreatedAt: time.Now(),
	}))

	now := time.Now().Unix()
	insertAgentAlert := func(id, agentID, name string) {
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO alerts (id, source, alert_type, severity, status, message,
			 entity_type, entity_id, entity_name, fired_at, created_at)
			 VALUES (?,'agent','disconnected','critical','active','disconnected','agent',?,?,?,?)`,
			id, agentID, name, now, now,
		)
		require.NoError(t, err)
	}
	insertAgentAlert("alert-orphan", "deleted-agent", "ghost")
	insertAgentAlert("alert-live", "live-agent", "live")

	// Start loads both active alerts into the engine's in-memory map synchronously.
	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Start(engineCtx)
	require.Equal(t, 2, engine.AlertCount(), "both alerts should be active before pruning")

	a := &App{
		alertStore:  alertStore,
		agentStore:  agentStore,
		alertEngine: engine,
		logger:      logger,
	}
	a.pruneOrphanAlerts(ctx)

	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1, "the orphan agent alert must be resolved")
	assert.Equal(t, "live-agent", active[0].EntityID, "the alert of the still-enrolled agent must remain active")
	assert.Equal(t, 1, engine.AlertCount(), "engine in-memory map must drop the resolved alert")
}

// TestPruneOrphanAlerts_ResolvesDeletedMonitors covers the DB-backed monitors:
// an alert whose heartbeat, endpoint or certificate monitor no longer exists is
// resolved, while alerts of monitors that still exist are left untouched.
func TestPruneOrphanAlerts_ResolvesDeletedMonitors(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, store.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	alertStore := store.NewAlertStore(db)
	hbStore := store.NewHeartbeatStore(db)
	epStore := store.NewEndpointStore(db)
	certStore := store.NewCertificateStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})

	hbID, err := hbStore.CreateHeartbeat(ctx, &heartbeat.Heartbeat{Name: "backup", IntervalSeconds: 60, GraceSeconds: 30})
	require.NoError(t, err)
	epID, err := epStore.UpsertEndpoint(ctx, &endpoint.Endpoint{
		ContainerName: "web", LabelKey: "http", EndpointType: endpoint.TypeHTTP, Target: "http://web/health",
	})
	require.NoError(t, err)
	certID, err := certStore.CreateMonitor(ctx, &certificate.CertMonitor{
		Hostname: "example.com", Port: 443, Source: certificate.SourceStandalone,
		Status: certificate.StatusValid, CheckIntervalSeconds: 3600, WarningThresholds: []int{30, 7},
	})
	require.NoError(t, err)

	now := time.Now().Unix()
	insertAlert := func(id, source, alertType, entityType, entityID string) {
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO alerts (id, source, alert_type, severity, status, message,
			 entity_type, entity_id, entity_name, fired_at, created_at)
			 VALUES (?,?,?,'critical','active','down',?,?,'name',?,?)`,
			id, source, alertType, entityType, entityID, now, now,
		)
		require.NoError(t, err)
	}
	insertAlert("hb-live", "heartbeat", "deadline_missed", "heartbeat", hbID)
	insertAlert("hb-orphan", "heartbeat", "deadline_missed", "heartbeat", "deleted-heartbeat")
	insertAlert("ep-live", "endpoint", "consecutive_failure", "endpoint", epID)
	insertAlert("ep-orphan", "endpoint", "consecutive_failure", "endpoint", "deleted-endpoint")
	insertAlert("cert-live", "certificate", "expiring", "certificate", certID)
	insertAlert("cert-orphan", "certificate", "expiring", "certificate", "deleted-certificate")

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Start(engineCtx)
	require.Equal(t, 6, engine.AlertCount())

	a := &App{
		alertStore:  alertStore,
		hbStore:     hbStore,
		epStore:     epStore,
		certStore:   certStore,
		alertEngine: engine,
		logger:      logger,
	}
	a.pruneOrphanAlerts(ctx)

	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	var remaining []string
	for _, al := range active {
		remaining = append(remaining, al.ID)
	}
	assert.ElementsMatch(t, []string{"hb-live", "ep-live", "cert-live"}, remaining)
	assert.Equal(t, 3, engine.AlertCount())
}
