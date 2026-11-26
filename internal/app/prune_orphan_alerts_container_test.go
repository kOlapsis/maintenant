// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

type unreadableContainerStore struct {
	container.ContainerStore
	unreadable string
}

func (s unreadableContainerStore) GetContainerByID(ctx context.Context, id string) (*container.Container, error) {
	if id == s.unreadable {
		return nil, errors.New("read failed")
	}
	return s.ContainerStore.GetContainerByID(ctx, id)
}

func TestPruneOrphanAlerts_KeepsContainerAlertOnReadError(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	containerStore := store.NewContainerStore(db)
	alertStore := store.NewAlertStore(db)
	engine := alert.NewEngine(alert.EngineDeps{
		AlertStore:   alertStore,
		ChannelStore: store.NewChannelStore(db),
		TriggerStore: store.NewTriggerStore(db),
		SilenceStore: store.NewSilenceStore(db),
		Logger:       logger,
	})

	liveID, err := containerStore.InsertContainer(ctx, &container.Container{
		ExternalID: "ext-live", Name: "web", Image: "nginx:latest", State: container.StateRunning,
		AlertSeverity: container.SeverityWarning, RestartThreshold: 3,
		FirstSeenAt: time.Now(), LastStateChangeAt: time.Now(),
	})
	require.NoError(t, err)

	for _, entityID := range []string{liveID, "removed-container"} {
		_, err := alertStore.InsertAlert(ctx, &alert.Alert{
			Source: alert.SourceContainer, AlertType: restartLoopAlertType, Severity: alert.SeverityCritical,
			Status: alert.StatusActive, Message: "restart loop", EntityType: "container", EntityID: entityID,
			EntityName: "web", Details: "{}", FiredAt: time.Now(),
		})
		require.NoError(t, err)
	}

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Start(engineCtx)

	a := &App{
		alertStore: alertStore,
		containerSvc: container.NewService(container.Deps{
			Store:  unreadableContainerStore{ContainerStore: containerStore, unreadable: liveID},
			Logger: logger,
		}),
		alertEngine: engine,
		logger:      logger,
	}
	a.pruneOrphanAlerts(ctx)

	active, err := alertStore.ListActiveAlerts(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, liveID, active[0].EntityID)
}
