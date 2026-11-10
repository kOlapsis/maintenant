// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkRecorder struct {
	mu     sync.Mutex
	counts map[string]int
}

func newCheckRecorder() *checkRecorder { return &checkRecorder{counts: map[string]int{}} }

func (r *checkRecorder) record(id string) {
	r.mu.Lock()
	r.counts[id]++
	r.mu.Unlock()
}

func (r *checkRecorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[id]
}

func fastConfig(interval time.Duration) EndpointConfig {
	c := DefaultConfig()
	c.Interval = Duration(interval)
	c.Timeout = Duration(2 * time.Second)
	return c
}

func seedStandalone(t *testing.T, store *memStore, target string, cfg EndpointConfig, active bool) string {
	t.Helper()
	id, err := store.InsertStandaloneEndpoint(context.Background(), &Endpoint{
		Name: "sa-" + target, Target: target, EndpointType: TypeHTTP, Config: cfg,
	})
	require.NoError(t, err)
	if !active {
		require.NoError(t, store.DeactivateEndpoint(context.Background(), id))
	}
	return id
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	t.Cleanup(s.Close)
	return s
}

func eventually(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, d, 20*time.Millisecond, msg)
}

func TestService_Start_ResumesStandaloneChecks(t *testing.T) {
	srv := okServer(t)
	store := newMemStore()
	ids := []string{
		seedStandalone(t, store, srv.URL+"/a", fastConfig(100*time.Millisecond), true),
		seedStandalone(t, store, srv.URL+"/b", fastConfig(150*time.Millisecond), true),
		seedStandalone(t, store, srv.URL+"/c", fastConfig(120*time.Millisecond), true),
	}
	rec := newCheckRecorder()
	engine := NewCheckEngine(func(id string, _ CheckResult) { rec.record(id) }, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})

	require.Equal(t, 0, engine.ActiveCount())
	svc.Start(context.Background())

	assert.Equal(t, 3, engine.ActiveCount(), "every active standalone endpoint is resumed")
	for _, id := range ids {
		id := id
		eventually(t, 3*time.Second, func() bool { return rec.count(id) >= 2 },
			"endpoint "+id+" must be checked at start and then again at its interval")
	}
}

func TestService_Start_SkipsInactiveDeletedAndLabel(t *testing.T) {
	srv := okServer(t)
	store := newMemStore()
	active := seedStandalone(t, store, srv.URL+"/on", fastConfig(100*time.Millisecond), true)
	inactive := seedStandalone(t, store, srv.URL+"/off", fastConfig(100*time.Millisecond), false)
	deleted := seedStandalone(t, store, srv.URL+"/del", fastConfig(100*time.Millisecond), true)
	store.mu.Lock()
	delete(store.endpoints, deleted)
	store.mu.Unlock()
	label := seedEndpoint(t, store, &Endpoint{
		ExternalID: "ctr", LabelKey: "k", ContainerName: "c", Target: srv.URL + "/label",
		EndpointType: TypeHTTP, Config: fastConfig(100 * time.Millisecond), Active: true,
	})

	rec := newCheckRecorder()
	engine := NewCheckEngine(func(id string, _ CheckResult) { rec.record(id) }, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	svc.Start(context.Background())

	assert.Equal(t, 1, engine.ActiveCount(), "only the active standalone endpoint is resumed")
	eventually(t, 3*time.Second, func() bool { return rec.count(active) >= 1 }, "active endpoint is checked")
	time.Sleep(350 * time.Millisecond)
	assert.Zero(t, rec.count(inactive), "inactive endpoint must not be checked")
	assert.Zero(t, rec.count(deleted), "deleted endpoint must not be checked")
	assert.Zero(t, rec.count(label), "label endpoints are left to container reconciliation")
}

func TestService_Start_NoDuplicateChecks(t *testing.T) {
	srv := okServer(t)
	store := newMemStore()
	interval := 200 * time.Millisecond
	id := seedStandalone(t, store, srv.URL, fastConfig(interval), true)

	rec := newCheckRecorder()
	engine := NewCheckEngine(func(id string, _ CheckResult) { rec.record(id) }, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	ctx := context.Background()
	svc.Start(ctx)

	require.Equal(t, 1, engine.ActiveCount(), "Start resumed the endpoint")
	ep, err := store.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	engine.AddEndpoint(ctx, ep)
	assert.Equal(t, 1, engine.ActiveCount(), "one runner per endpoint")

	time.Sleep(1100 * time.Millisecond)
	// Two runners would give about 12 checks; one runner gives 6-7.
	n := rec.count(id)
	assert.GreaterOrEqual(t, n, 3)
	assert.LessOrEqual(t, n, 8, "an endpoint is checked once per interval, not once per runner")
}

type failingListStore struct{ *memStore }

func (f failingListStore) ListEndpoints(context.Context, ListEndpointsOpts) ([]*Endpoint, error) {
	return nil, errors.New("db unreadable")
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestService_Start_LogsReadFailureAsError(t *testing.T) {
	buf := &lockedBuf{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	engine := NewCheckEngine(nil, logger)
	defer engine.Stop()
	svc := NewService(Deps{Store: failingListStore{newMemStore()}, Engine: engine, Logger: logger})

	done := make(chan struct{})
	go func() { svc.Start(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start must return even when the store cannot be read")
	}

	assert.Equal(t, 0, engine.ActiveCount())
	var errLine string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, "level=ERROR") && strings.Contains(l, "db unreadable") {
			errLine = l
		}
	}
	assert.NotEmpty(t, errLine, "the read failure must be logged at ERROR level with its cause; got:\n%s", buf.String())
}

func TestService_Start_DetectsOutageAfterRestart(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	for _, threshold := range []int{2, 3} {
		store := newMemStore()
		cfg := fastConfig(60 * time.Millisecond)
		cfg.Timeout = Duration(500 * time.Millisecond)
		cfg.FailureThreshold = threshold
		cfg.RecoveryThreshold = 1
		id, err := store.InsertStandaloneEndpoint(context.Background(), &Endpoint{
			Name: "tcp", Target: addr, EndpointType: TypeTCP, Config: cfg, Status: StatusUp, AlertState: AlertNormal,
		})
		require.NoError(t, err)

		var svc *Service
		var alerts atomic.Int32
		engine := NewCheckEngine(func(id string, r CheckResult) {
			svc.ProcessCheckResult(context.Background(), id, r)
		}, noopLogger())
		svc = NewService(Deps{Store: store, Engine: engine, Logger: noopLogger(),
			AlertCallback: func(ep *Endpoint, _ CheckResult) (string, interface{}) {
				if ep.AlertState == AlertNormal && ep.ConsecutiveFailures >= ep.Config.FailureThreshold {
					alerts.Add(1)
					return "endpoint.alert", nil
				}
				return "", nil
			}})
		svc.Start(context.Background())

		eventually(t, 5*time.Second, func() bool {
			ep, _ := store.GetEndpointByID(context.Background(), id)
			return ep != nil && ep.Status == StatusDown && ep.AlertState == AlertAlerting
		}, "endpoint must go down and alert once the failure threshold is reached")
		ep, _ := store.GetEndpointByID(context.Background(), id)
		assert.GreaterOrEqual(t, ep.ConsecutiveFailures, threshold)
		engine.Stop()
	}
}

func TestService_Start_ThenUpdateUsesNewTarget(t *testing.T) {
	var oldHits, newHits atomic.Int32
	oldSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { oldHits.Add(1) }))
	defer oldSrv.Close()
	newSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { newHits.Add(1) }))
	defer newSrv.Close()

	store := newMemStore()
	cfg := fastConfig(80 * time.Millisecond)
	id := seedStandalone(t, store, oldSrv.URL, cfg, true)

	engine := NewCheckEngine(nil, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	ctx := context.Background()
	svc.Start(ctx)
	eventually(t, 3*time.Second, func() bool { return oldHits.Load() >= 2 }, "old target is checked after restart")

	_, err := svc.UpdateStandalone(ctx, id, "renamed", newSrv.URL, TypeHTTP, cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, engine.ActiveCount(), "update replaces the runner")

	eventually(t, 3*time.Second, func() bool { return newHits.Load() >= 2 }, "new target is checked")
	time.Sleep(150 * time.Millisecond)
	before := oldHits.Load()
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, before, oldHits.Load(), "the old configuration is no longer checked")
}

func TestService_Start_ThenDeleteStopsChecks(t *testing.T) {
	srv := okServer(t)
	store := newMemStore()
	id := seedStandalone(t, store, srv.URL, fastConfig(80*time.Millisecond), true)
	engine := NewCheckEngine(nil, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	ctx := context.Background()
	svc.Start(ctx)
	require.Equal(t, 1, engine.ActiveCount())

	require.NoError(t, svc.Delete(ctx, id))
	assert.Equal(t, 0, engine.ActiveCount())
}

func TestService_Start_ResumesHundreds(t *testing.T) {
	pinEdition(t, extension.Pro)
	store := newMemStore()
	for i := 0; i < 300; i++ {
		seedStandalone(t, store, "http://127.0.0.1:1/"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+time.Duration(i).String(), fastConfig(time.Hour), true)
	}
	engine := NewCheckEngine(nil, noopLogger())
	defer engine.Stop()
	svc := NewService(Deps{Store: store, Engine: engine, Logger: noopLogger()})
	svc.Start(context.Background())
	assert.Equal(t, 300, engine.ActiveCount(), "none is forgotten")
}
