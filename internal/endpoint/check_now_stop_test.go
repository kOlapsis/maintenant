// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

func TestService_StopCancelsAndWaitsForAManualCheck(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	store := newMemStore()
	svc := NewService(Deps{Store: ctxStore{store}, Engine: noopEngine(), Logger: noopLogger()})
	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "ctr-stop", LabelKey: "k1",
		EndpointType: TypeHTTP, Target: srv.URL,
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
		AgentID: uid.LocalAgent,
	})

	checked := make(chan error, 1)
	go func() {
		_, err := svc.CheckNow(context.Background(), id)
		checked <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the probe never reached the target")
	}

	svc.Stop()
	select {
	case err := <-checked:
		assert.Error(t, err, "a probe cut short by the stop is not a result")
	case <-time.After(2 * time.Second):
		t.Fatal("Stop returned while the manual check was still running")
	}
	_, err := svc.CheckNow(context.Background(), id)
	require.Error(t, err, "no manual check starts once the service is stopped")

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Empty(t, store.results)
}
