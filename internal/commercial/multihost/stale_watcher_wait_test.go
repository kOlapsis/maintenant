// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost_test

import (
	"context"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/commercial/multihost"
)

func TestSessions_StaleWatcher_ReturnsOnlyOnceItsSweepHasReturned(t *testing.T) {
	sessions := multihost.NewSessions(discardLogger(), &testBroadcaster{})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	staleAgents := func(context.Context, time.Duration) ([]string, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		sessions.StartStaleWatcher(ctx, time.Millisecond, time.Minute, 0, staleAgents)
		close(returned)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the watcher never queried the store")
	}

	cancel()
	select {
	case <-returned:
		close(release)
		t.Fatal("the watcher returned while its store query was still running")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher did not return once its context ended")
	}
}
