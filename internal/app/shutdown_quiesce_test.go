// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShutdown_WaitsForBackgroundLoopsBeforeClosingTheDatabase(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	require.Eventually(t, func() bool { return instanceRows(t, cfg.DBPath, a.instanceID) == 1 }, 10*time.Second, 20*time.Millisecond)

	draining := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Bool
	a.background.Go(func() {
		<-ctx.Done()
		close(draining)
		<-release
		finished.Store(true)
	})

	cancel()
	<-draining
	select {
	case <-done:
		t.Fatal("Start returned while a background loop was still running")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	require.True(t, finished.Load(), "the loop finished before Start returned")
}

func TestShutdown_BackgroundWaitIsBoundedByTheGrace(t *testing.T) {
	cfg, _, logger := storageEnv(t)
	a, err := New(cfg, logger)
	require.NoError(t, err)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	a.background.Go(func() { <-release })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	waited := make(chan struct{})
	go func() {
		a.waitBackground(ctx)
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("waitBackground ignored its deadline")
	}
}
