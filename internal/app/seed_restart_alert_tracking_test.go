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
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// stubRestartChecker always reports the configured result, regardless of the
// container passed in.
type stubRestartChecker struct {
	result interface{}
}

func (c *stubRestartChecker) Check(_ context.Context, _ *container.Container) (interface{}, error) {
	return c.result, nil
}

// TestSeedRestartAlertTracking_ResumesRecoveryAfterRestart covers the
// server-restart case: a restart_loop alert persisted before the process
// restarted is reloaded into the container service's recovery tracker, so a
// container that stabilised in the meantime still gets its alert resolved.
func TestSeedRestartAlertTracking_ResumesRecoveryAfterRestart(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	containerStore := store.NewContainerStore(db)
	alertStore := store.NewAlertStore(db)

	c := &container.Container{
		ExternalID:        "restart-seed-external",
		Name:              "web",
		Image:             "nginx:latest",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		FirstSeenAt:       time.Now(),
		LastStateChangeAt: time.Now(),
	}
	id, err := containerStore.InsertContainer(ctx, c)
	require.NoError(t, err)

	_, err = alertStore.InsertAlert(ctx, &alert.Alert{
		Source:     alert.SourceContainer,
		AlertType:  restartLoopAlertType,
		Severity:   alert.SeverityCritical,
		Status:     alert.StatusActive,
		Message:    "restart loop",
		EntityType: "container",
		EntityID:   id,
		EntityName: "web",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)

	var mu sync.Mutex
	var emitted []string
	checker := &stubRestartChecker{result: nil}
	svc := container.NewService(container.Deps{
		Store:                   containerStore,
		Logger:                  logger,
		RestartChecker:          checker,
		RestartRecoveryInterval: 5 * time.Millisecond,
		EventCallback: func(eventType string, _ interface{}) {
			mu.Lock()
			defer mu.Unlock()
			emitted = append(emitted, eventType)
		},
	})

	a := &App{alertStore: alertStore, containerSvc: svc, logger: logger}
	a.seedRestartAlertTracking(ctx)

	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.RunRestartRecoveryLoop(loopCtx)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(emitted, "container.restart_recovery")
	}, time.Second, 5*time.Millisecond, "seeding must resume the recovery timer for the stabilised container")
}

// TestSeedRestartAlertTracking_IgnoresOtherAlertTypes verifies only active
// restart_loop alerts on containers are used to seed the tracker.
func TestSeedRestartAlertTracking_IgnoresOtherAlertTypes(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	containerStore := store.NewContainerStore(db)
	alertStore := store.NewAlertStore(db)

	c := &container.Container{
		ExternalID:        "other-alert-external",
		Name:              "web",
		Image:             "nginx:latest",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		FirstSeenAt:       time.Now(),
		LastStateChangeAt: time.Now(),
	}
	id, err := containerStore.InsertContainer(ctx, c)
	require.NoError(t, err)

	_, err = alertStore.InsertAlert(ctx, &alert.Alert{
		Source:     alert.SourceContainer,
		AlertType:  "health_unhealthy",
		Severity:   alert.SeverityWarning,
		Status:     alert.StatusActive,
		Message:    "unhealthy",
		EntityType: "container",
		EntityID:   id,
		EntityName: "web",
		Details:    "{}",
		FiredAt:    time.Now(),
	})
	require.NoError(t, err)

	var mu sync.Mutex
	var emitted []string
	checker := &stubRestartChecker{result: nil}
	svc := container.NewService(container.Deps{
		Store:                   containerStore,
		Logger:                  logger,
		RestartChecker:          checker,
		RestartRecoveryInterval: 5 * time.Millisecond,
		EventCallback: func(eventType string, _ interface{}) {
			mu.Lock()
			defer mu.Unlock()
			emitted = append(emitted, eventType)
		},
	})

	a := &App{alertStore: alertStore, containerSvc: svc, logger: logger}
	a.seedRestartAlertTracking(ctx)

	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.RunRestartRecoveryLoop(loopCtx)

	time.Sleep(30 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, emitted, "a non restart_loop alert must not seed the recovery tracker")
}
