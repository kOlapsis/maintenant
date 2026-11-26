// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestShutdown_DeregistersWhenTheWaitsUsedTheWholeGrace(t *testing.T) {
	prev := shutdownGrace
	shutdownGrace = 100 * time.Millisecond
	t.Cleanup(func() { shutdownGrace = prev })

	cfg, logs, logger := storageEnv(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	require.Eventually(t, func() bool { return instanceRows(t, cfg.DBPath, a.instanceID) == 1 }, 10*time.Second, 20*time.Millisecond)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	a.background.Go(func() { <-release })

	cancel()
	require.NoError(t, <-done)
	assert.True(t, strings.Contains(logs.String(), "background loops still running at shutdown"), "the waits used the whole grace")
	assert.Zero(t, instanceRows(t, cfg.DBPath, a.instanceID), "the next start would report this run as a peer")
}

func TestShutdown_AlertProducerGivesUpOnAFullChannel(t *testing.T) {
	cfg, _, _ := storageEnv(t)
	h := newHold()
	t.Cleanup(h.let)
	logs := &syncBuffer{}
	logger := slog.New(holdingHandler{
		Handler: slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
		msg:     "alert: dedup, skipping duplicate",
		h:       h,
	})
	a, err := New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	require.Eventually(t, func() bool { return strings.Contains(logs.String(), "starting HTTP server") },
		10*time.Second, 10*time.Millisecond)

	evt := alert.Event{Source: alert.SourceEndpoint, AlertType: "shutdown_probe", Severity: alert.SeverityWarning,
		EntityType: "endpoint", EntityID: "probe", EntityName: "probe", Message: "probe", Timestamp: time.Now()}
	ch := a.alertEngine.EventChannel()
	ch <- evt
	ch <- evt
	select {
	case <-h.held:
	case <-time.After(10 * time.Second):
		t.Fatal("the engine never reached its hold")
	}
fill:
	for {
		select {
		case ch <- evt:
		default:
			break fill
		}
	}

	returned := make(chan struct{})
	go func() {
		a.emitAlert(evt)
		close(returned)
	}()
	cancel()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the producer stayed blocked on the full alert channel after the app stopped")
	}
	h.let()
	require.NoError(t, <-done)
}
