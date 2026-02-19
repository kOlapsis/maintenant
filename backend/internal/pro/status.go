package pro

import (
	"context"
	"time"

	"github.com/kolapsis/pulseboard/internal/alert"
)

// StatusIncident is a minimal representation for the status page API response.
type StatusIncident struct {
	ID        int64
	Title     string
	Severity  string
	Status    string
	CreatedAt time.Time
	Updates   []StatusIncidentUpdate
}

// StatusIncidentUpdate is a timestamped entry in an incident timeline.
type StatusIncidentUpdate struct {
	Status    string
	Message   string
	CreatedAt time.Time
}

// StatusMaintenanceWindow is a minimal representation for the status page API response.
type StatusMaintenanceWindow struct {
	ID          int64
	Title       string
	Description string
	StartsAt    time.Time
	EndsAt      time.Time
	Active      bool
}

// IncidentManager handles incident lifecycle on the status page.
// CE: no-op. Pro: full incident CRUD + auto-incident from alerts.
type IncidentManager interface {
	HandleAlertEvent(ctx context.Context, evt alert.Event) error
	ListActiveIncidents(ctx context.Context) ([]StatusIncident, error)
	ListRecentIncidents(ctx context.Context, limit int) ([]StatusIncident, error)
}

// SubscriberNotifier sends email notifications to status page subscribers.
// CE: no-op. Pro: SMTP delivery.
type SubscriberNotifier interface {
	NotifyAll(ctx context.Context, subject string, htmlBody string) error
}

// MaintenanceScheduler manages scheduled maintenance windows on the status page.
// CE: no-op. Pro: auto-activate/deactivate windows, create incidents, notify subscribers.
type MaintenanceScheduler interface {
	Start(ctx context.Context) error
	ListUpcoming(ctx context.Context) ([]StatusMaintenanceWindow, error)
}

// NoopIncidentManager is the CE default.
type NoopIncidentManager struct{}

func (NoopIncidentManager) HandleAlertEvent(_ context.Context, _ alert.Event) error {
	return nil
}
func (NoopIncidentManager) ListActiveIncidents(_ context.Context) ([]StatusIncident, error) {
	return nil, nil
}
func (NoopIncidentManager) ListRecentIncidents(_ context.Context, _ int) ([]StatusIncident, error) {
	return nil, nil
}

// NoopSubscriberNotifier is the CE default.
type NoopSubscriberNotifier struct{}

func (NoopSubscriberNotifier) NotifyAll(_ context.Context, _ string, _ string) error {
	return nil
}

// NoopMaintenanceScheduler is the CE default.
type NoopMaintenanceScheduler struct{}

func (NoopMaintenanceScheduler) Start(_ context.Context) error { return nil }
func (NoopMaintenanceScheduler) ListUpcoming(_ context.Context) ([]StatusMaintenanceWindow, error) {
	return nil, nil
}
