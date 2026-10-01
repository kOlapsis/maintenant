// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8s "k8s.io/client-go/kubernetes"
)

// fakeAPIServer answers /version with 503 for the first failures calls, then with a version.
func fakeAPIServer(t *testing.T, failures int32) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		if hits.Add(1) <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"major":"1","minor":"30","gitVersion":"v1.30.0"}`))
	}))
	t.Cleanup(srv.Close)

	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	cfg := "apiVersion: v1\nkind: Config\n" +
		"clusters:\n- name: fake\n  cluster:\n    server: " + srv.URL + "\n" +
		"users:\n- name: fake\n  user:\n    token: fake\n" +
		"contexts:\n- name: fake\n  context:\n    cluster: fake\n    user: fake\n" +
		"current-context: fake\n"
	require.NoError(t, os.WriteFile(kubeconfig, []byte(cfg), 0o600))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", kubeconfig)
	return &hits
}

func withClient(r *Runtime, cs k8s.Interface) *Runtime {
	r.conn.Store(&clients{core: cs})
	return r
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	r, err := NewRuntime(slog.New(slog.NewTextHandler(io.Discard, nil)), NewNamespaceFilter("", ""))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestConnect_RetriesUntilTheAPIServerAnswers(t *testing.T) {
	hits := fakeAPIServer(t, 1)
	r := newTestRuntime(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, r.Connect(ctx))

	assert.True(t, r.IsConnected())
	assert.Equal(t, int32(2), hits.Load())
}

func TestConnect_StopsWithItsContext(t *testing.T) {
	fakeAPIServer(t, 1<<30)
	r := newTestRuntime(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	assert.ErrorIs(t, r.Connect(ctx), context.DeadlineExceeded)
	assert.False(t, r.IsConnected())
}

// Run under -race: logs, topology and stats read the clients while reconnections replace them.
func TestReconnect_WhileReading(t *testing.T) {
	fakeAPIServer(t, 0)
	r := newTestRuntime(t)
	ctx := context.Background()
	require.NoError(t, r.TryConnect(ctx))

	readers := []func(){
		func() { _, _ = r.FetchLogs(ctx, "default/web", 10, false) },
		func() { _, _ = SnapshotFromRuntime(ctx, r) },
		func() { _, _ = r.StatsSnapshot(ctx, "default/web") },
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, read := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					read()
				}
			}
		}()
	}
	for range 20 {
		require.NoError(t, r.TryConnect(ctx))
	}
	close(done)
	wg.Wait()
}

func TestTryConnect_MakesASingleAttempt(t *testing.T) {
	hits := fakeAPIServer(t, 1)
	r := newTestRuntime(t)

	require.Error(t, r.TryConnect(context.Background()))

	assert.False(t, r.IsConnected())
	assert.Equal(t, int32(1), hits.Load())
}
