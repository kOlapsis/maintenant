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
	action, err := NoopEscalator{}.Evaluate(context.Background(), "alert-1", 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != nil {
		t.Fatal("expected nil action")
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

func TestNoopTemplateEngine(t *testing.T) {
	result, err := NoopTemplateEngine{}.Render(context.Background(), "test", map[string]any{"key": "val"})
	if err != ErrNotAvailable {
		t.Fatalf("expected ErrNotAvailable, got %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
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
