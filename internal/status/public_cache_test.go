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
	"time"

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

type cancellingIncidentStore struct {
	IncidentStore
	cancel context.CancelFunc
	wait   bool
	calls  atomic.Int64
}

func (s *cancellingIncidentStore) ListActiveIncidents(ctx context.Context) ([]Incident, error) {
	s.calls.Add(1)
	s.cancel()
	if s.wait {
		<-ctx.Done()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []Incident{{ID: "ongoing"}}, nil
}

func cancellingService(incidents IncidentStore) *Service {
	return NewService(Deps{Components: emptyComponentStore{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Incidents: incidents})
}

func TestPublicPageData_ACancelledVisitorDoesNotDegradeTheSharedSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := cancellingService(&cancellingIncidentStore{cancel: cancel})

	_, _ = svc.PublicPageData(ctx)

	next, err := svc.PublicPageData(context.Background())
	if err != nil {
		t.Fatalf("next visitor: %v", err)
	}
	if len(next.ActiveIncidents) != 1 {
		t.Fatalf("the next visitor got %d incidents, want the ongoing one", len(next.ActiveIncidents))
	}
}

func TestPublicPageData_ALoadThatRanOutOfTimeIsNotKept(t *testing.T) {
	prev := publicLoadTimeout
	publicLoadTimeout = 10 * time.Millisecond
	t.Cleanup(func() { publicLoadTimeout = prev })

	incidents := &cancellingIncidentStore{cancel: func() {}, wait: true}
	svc := cancellingService(incidents)

	if got, err := svc.PublicPageData(context.Background()); err == nil {
		t.Fatalf("got %+v from a load that ran out of time, want an error", got)
	}
	_, _ = svc.PublicPageData(context.Background())
	if got := incidents.calls.Load(); got != 2 {
		t.Fatalf("%d loads, want 2: the first one must not have been kept", got)
	}
}
