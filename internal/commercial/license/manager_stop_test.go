// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

func TestManager_StopWaitsForTheEditionChangeCallbacks(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	var refused atomic.Bool
	m := testManager(t, pub, func(w http.ResponseWriter, _ *http.Request) {
		if refused.Load() {
			writeSignedRefusal(t, w, priv, http.StatusUnauthorized, LicensePayload{Message: "unknown key"})
			return
		}
		_ = json.NewEncoder(w).Encode(signPayload(t, priv, proPayload()))
	})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	m.RegisterEditionChangeCallback(func(context.Context, extension.Edition, extension.Edition) {
		entered <- struct{}{}
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	refused.Store(true)
	m.check(ctx)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the edition change was never dispatched")
	}

	cancel()
	stopped := make(chan struct{})
	go func() {
		m.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop returned while an edition change callback was still running")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return once the callback had")
	}
}
