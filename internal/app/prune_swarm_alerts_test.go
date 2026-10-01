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

func TestPruneSwarmAlerts_ResolvesServicesAndNodesNoLongerListed(t *testing.T) {
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
	insert := func(id, alertType, entityType, entityID string) {
		_, err := db.Writer().Exec(ctx,
			`INSERT INTO alerts (id, source, alert_type, severity, status, message,
			 entity_type, entity_id, entity_name, fired_at, created_at)
			 VALUES (?,'swarm',?,'warning','active','down',?,?,'name',?,?)`,
			id, alertType, entityType, entityID, now, now,
		)
		require.NoError(t, err)
	}
	insert("live", "replica_unhealthy", "swarm_service", "svc1")
	insert("removed", "replica_unhealthy", "swarm_service", "svc-removed")
	insert("legacy", "update_rollback", "swarm_service", "")
	insert("node-live", "node_down", "swarm_node", "n1")
	insert("node-removed", "node_down", "swarm_node", "n-removed")
	insert("node-legacy", "node_drain", "swarm_node", "")

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Start(engineCtx)

	a := &App{alertStore: alertStore, alertEngine: engine, logger: logger}

	a.pruneSwarmAlerts(ctx, swarm.TopologySnapshot{Services: []swarm.SwarmService{{ServiceID: "svc1"}}})
	assert.ElementsMatch(t, []string{"live", "node-live", "node-removed", "node-legacy"}, activeIDs(t, ctx, alertStore),
		"nodes that could not be listed resolve nothing")

	a.pruneSwarmAlerts(ctx, swarm.TopologySnapshot{
		Services: []swarm.SwarmService{{ServiceID: "svc1"}},
		Nodes:    []swarm.SwarmNode{{NodeID: "n1"}},
	})
	assert.ElementsMatch(t, []string{"live", "node-live"}, activeIDs(t, ctx, alertStore))
	assert.Equal(t, 2, engine.AlertCount())
}

func activeIDs(t *testing.T, ctx context.Context, alertStore *store.AlertStoreImpl) []string {
	t.Helper()
	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	ids := make([]string, 0, len(active))
	for _, al := range active {
		ids = append(ids, al.ID)
	}
	return ids
}
