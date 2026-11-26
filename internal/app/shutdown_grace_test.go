// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
