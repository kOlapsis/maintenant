// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/swarm"
)

func TestPruneSwarmServiceAlerts_ResolvesServicesNoLongerListed(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	alertStore := store.NewAlertStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})

	now := time.Now().Unix()
	insert := func(id, alertType, entityID string) {
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO alerts (id, source, alert_type, severity, status, message,
			 entity_type, entity_id, entity_name, fired_at, created_at)
			 VALUES (?,'swarm',?,'warning','active','down','swarm_service',?,'name',?,?)`,
			id, alertType, entityID, now, now,
		)
		require.NoError(t, err)
	}
	insert("live", "replica_unhealthy", "svc1")
	insert("removed", "replica_unhealthy", "svc-removed")
	insert("legacy", "update_rollback", "")

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Start(engineCtx)

	a := &App{alertStore: alertStore, alertEngine: engine, logger: logger}
	a.pruneSwarmServiceAlerts(ctx, []swarm.SwarmService{{ServiceID: "svc1"}})

	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "live", active[0].ID)
	assert.Equal(t, 1, engine.AlertCount())
}
