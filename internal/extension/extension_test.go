// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package extension

import (
	"context"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestCurrentEditionReturnsCommunity(t *testing.T) {
	if got := CurrentEdition(); got != Community {
		t.Fatalf("expected Community, got %s", got)
	}
}

func TestErrNotAvailable(t *testing.T) {
	if ErrNotAvailable == nil {
		t.Fatal("ErrNotAvailable should not be nil")
	}
}

func TestNoopEscalator(t *testing.T) {
	ctx := context.Background()
	n := NoopEscalator{}

	if err := n.EvaluateCycle(ctx); err != nil {
		t.Fatalf("EvaluateCycle: unexpected error: %v", err)
	}
	if err := n.OnAlertCreated(ctx, &alert.Alert{ID: "1"}); err != nil {
		t.Fatalf("OnAlertCreated: unexpected error: %v", err)
	}
	if err := n.OnAlertAcknowledged(ctx, "1", alert.Acknowledgment{By: "alice", At: time.Now()}); err != nil {
		t.Fatalf("OnAlertAcknowledged: unexpected error: %v", err)
	}
	if err := n.OnAlertResolved(ctx, "1", time.Now()); err != nil {
		t.Fatalf("OnAlertResolved: unexpected error: %v", err)
	}
	if err := n.OnEditionDowngraded(ctx); err != nil {
		t.Fatalf("OnEditionDowngraded: unexpected error: %v", err)
	}
}

func TestNoopEntityRouter(t *testing.T) {
	channels, err := NoopEntityRouter{}.Route(context.Background(), "container", "c-1", "critical")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if channels != nil {
		t.Fatal("expected nil channels")
	}
}

func TestNoopMaintenanceSuppressor(t *testing.T) {
	suppressed, err := NoopMaintenanceSuppressor{}.IsSuppressed(context.Background(), "update", "container", "c-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suppressed {
		t.Fatal("expected not suppressed")
	}
}

func TestNoopIncidentManager(t *testing.T) {
	ctx := context.Background()
	m := NoopIncidentManager{}

	if err := m.HandleAlertEvent(ctx, alert.Event{}); err != nil {
		t.Fatalf("HandleAlertEvent: unexpected error: %v", err)
	}

	incidents, err := m.ListActiveIncidents(ctx)
	if err != nil {
		t.Fatalf("ListActiveIncidents: unexpected error: %v", err)
	}
	if incidents != nil {
		t.Fatal("expected nil incidents")
	}

	recent, err := m.ListRecentIncidents(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentIncidents: unexpected error: %v", err)
	}
	if recent != nil {
		t.Fatal("expected nil recent incidents")
	}
}

func TestNoopSubscriberNotifier(t *testing.T) {
	if err := (NoopSubscriberNotifier{}).NotifyAll(context.Background(), "subject", "<p>body</p>"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoopMaintenanceScheduler(t *testing.T) {
	ctx := context.Background()
	s := NoopMaintenanceScheduler{}

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	windows, err := s.ListUpcoming(ctx)
	if err != nil {
		t.Fatalf("ListUpcoming: unexpected error: %v", err)
	}
	if windows != nil {
		t.Fatal("expected nil windows")
	}
}
