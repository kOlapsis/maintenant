// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kolapsis/maintenant/internal/event"
)

type tallyIncidentStore struct {
	IncidentStore
	active, recent atomic.Int64
}

func (s *tallyIncidentStore) ListActiveIncidents(context.Context) ([]Incident, error) {
	s.active.Add(1)
	return nil, nil
}

func (s *tallyIncidentStore) ListRecentIncidents(context.Context, int) ([]Incident, error) {
	s.recent.Add(1)
	return nil, nil
}

func newCountingHandler() (*Handler, *Service, *tallyIncidentStore) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	incidents := &tallyIncidentStore{}
	svc := NewService(Deps{
		Components:        emptyComponentStore{},
		Logger:            logger,
		Incidents:         incidents,
		PublicBroadcaster: func(string, any) {},
	})
	return NewHandler(svc, nil, logger, nil, "https://status.example.com"), svc, incidents
}

func hit(t *testing.T, h http.HandlerFunc, target string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Errorf("%s: got %d, want 200", target, rec.Code)
	}
}

func TestStatusAPI_VisitorsShareOneSnapshot(t *testing.T) {
	h, svc, incidents := newCountingHandler()

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { hit(t, h.HandleStatusAPI, "/status/api") })
	}
	wg.Wait()
	if got := incidents.active.Load(); got != 1 {
		t.Fatalf("50 visitors computed the snapshot %d times, want 1", got)
	}

	svc.Broadcast(event.StatusIncidentCreated, nil)
	hit(t, h.HandleStatusAPI, "/status/api")
	if got := incidents.active.Load(); got != 2 {
		t.Fatalf("%d computations after a public event, want 2: the refetch it triggers must see the change", got)
	}

	svc.page.at = svc.page.at.Add(-publicCacheTTL)
	hit(t, h.HandleStatusAPI, "/status/api")
	if got := incidents.active.Load(); got != 3 {
		t.Fatalf("%d computations once the snapshot is %s old, want 3", got, publicCacheTTL)
	}
}

func TestAtomFeed_VisitorsShareOneIncidentList(t *testing.T) {
	h, svc, incidents := newCountingHandler()

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { hit(t, h.HandleAtomFeed, "/status/feed.atom") })
	}
	wg.Wait()
	if got := incidents.recent.Load(); got != 1 {
		t.Fatalf("50 readers loaded the incidents %d times, want 1", got)
	}

	svc.Broadcast(event.StatusIncidentCreated, nil)
	hit(t, h.HandleAtomFeed, "/status/feed.atom")
	if got := incidents.recent.Load(); got != 2 {
		t.Fatalf("%d loads after a public event, want 2", got)
	}
}
