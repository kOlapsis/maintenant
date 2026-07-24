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
	"github.com/kolapsis/maintenant/internal/store/sqlite"
)

// TestPruneOrphanAlerts_ResolvesDeletedAgent verifies that a disconnect alert
// for an agent that no longer exists is resolved on startup, while a disconnect
// alert for an agent that still exists is left untouched.
func TestPruneOrphanAlerts_ResolvesDeletedAgent(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, sqlite.Migrate(db.ReadDB(), logger))
	db.StartWriter(ctx)

	alertStore := sqlite.NewAlertStore(db)
	agentStore := sqlite.NewAgentStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: sqlite.NewChannelStore(db),
		TriggerStore: sqlite.NewTriggerStore(db),
		SilenceStore: sqlite.NewSilenceStore(db),
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
