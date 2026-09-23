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
package statuspage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponentStore struct {
	mu                     sync.Mutex
	visibleComponents      []status.Component
	visibleErr             error
	componentsByMonitor    []status.Component
	componentsByMonitorErr error
	removeDanglingCalls    []string // track calls: "type:id"
}

func (m *mockComponentStore) setComponentsByMonitor(comps []status.Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.componentsByMonitor = comps
}

func (m *mockComponentStore) ListVisibleComponents(ctx context.Context) ([]status.Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.visibleComponents, m.visibleErr
}

func (m *mockComponentStore) ListComponentsByMonitor(ctx context.Context, monitorType string, monitorID string) ([]status.Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.componentsByMonitorErr != nil {
		return nil, m.componentsByMonitorErr
	}
	return m.componentsByMonitor, nil
}

func (m *mockComponentStore) RemoveDanglingMonitorRefs(ctx context.Context, monitorType string, monitorID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeDanglingCalls = append(m.removeDanglingCalls, fmt.Sprintf("%s:%s", monitorType, monitorID))
	return nil
}

// Unused methods — satisfy interface with zero values.
func (m *mockComponentStore) ListComponents(ctx context.Context) ([]status.Component, error) {
	return nil, nil
}
func (m *mockComponentStore) GetComponent(ctx context.Context, id string) (*status.Component, error) {
	return nil, nil
}
func (m *mockComponentStore) CreateComponent(ctx context.Context, c *status.Component) (string, error) {
	return "", nil
}
func (m *mockComponentStore) UpdateComponent(ctx context.Context, c *status.Component) error {
	return nil
}
func (m *mockComponentStore) DeleteComponent(ctx context.Context, id string) error { return nil }

// mockIncidentStore implements IncidentStore. Call counts and arguments are
// captured so tests can assert what was called.
type mockIncidentStore struct {
	mu                   sync.Mutex
	activeByComponent    map[string]*status.Incident
	activeByComponentErr error
	createIncidentID     string
	createIncidentErr    error
	createIncidentCalls  []createIncidentCall
	createUpdateID       string
	createUpdateErr      error
	createUpdateCalls    []status.IncidentUpdate
	listActiveIncidents  []status.Incident
	listActiveErr        error
	listRecentIncidents  []status.Incident
	listRecentErr        error
}

type createIncidentCall struct {
	incident       status.Incident
	componentIDs   []string
	initialMessage string
}

func (m *mockIncidentStore) GetActiveIncidentByComponent(ctx context.Context, componentID string) (*status.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeByComponentErr != nil {
		return nil, m.activeByComponentErr
	}
	if m.activeByComponent == nil {
		return nil, nil
	}
	return m.activeByComponent[componentID], nil
}

func (m *mockIncidentStore) CreateIncident(ctx context.Context, inc *status.Incident, componentIDs []string, initialMessage string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createIncidentErr != nil {
		return "", m.createIncidentErr
	}
	m.createIncidentCalls = append(m.createIncidentCalls, createIncidentCall{
		incident:       *inc,
		componentIDs:   componentIDs,
		initialMessage: initialMessage,
	})
	return m.createIncidentID, nil
}

func (m *mockIncidentStore) CreateUpdate(ctx context.Context, u *status.IncidentUpdate) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createUpdateErr != nil {
		return "", m.createUpdateErr
	}
	m.createUpdateCalls = append(m.createUpdateCalls, *u)
	return m.createUpdateID, nil
}

func (m *mockIncidentStore) ListActiveIncidents(ctx context.Context) ([]status.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listActiveIncidents, m.listActiveErr
}

func (m *mockIncidentStore) ListRecentIncidents(ctx context.Context, days int) ([]status.Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listRecentIncidents, m.listRecentErr
}

// Unused methods.
func (m *mockIncidentStore) ListIncidents(ctx context.Context, opts status.ListIncidentsOpts) ([]status.Incident, int, error) {
	return nil, 0, nil
}
func (m *mockIncidentStore) GetIncident(ctx context.Context, id string) (*status.Incident, error) {
	return nil, nil
}
func (m *mockIncidentStore) UpdateIncident(ctx context.Context, inc *status.Incident, componentIDs []string) error {
	return nil
}
func (m *mockIncidentStore) DeleteIncident(ctx context.Context, id string) error { return nil }
func (m *mockIncidentStore) ListUpdates(ctx context.Context, incidentID string) ([]status.IncidentUpdate, error) {
	return nil, nil
}
func (m *mockIncidentStore) DeleteIncidentsOlderThan(ctx context.Context, days int) (int64, error) {
	return 0, nil
}

// --- Helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

func newTestService(cs status.ComponentStore, is status.IncidentStore) *status.Service {
	svc := status.NewService(status.Deps{
		Components: cs,
		Logger:     discardLogger(),
		Incidents:  is,
	})
	svc.SetIncidentHandler(NewIncidentHandler(cs, is, svc, discardLogger()))
	return svc
}

// makeExplicitComponent creates a component with explicit composition mode and one monitor.
func makeExplicitComponent(monitorType string, monitorID string) *status.Component {
	return &status.Component{
		ID:              "comp-10",
		DisplayName:     "API Gateway",
		CompositionMode: status.CompositionExplicit,
		Monitors:        []status.MonitorRef{{Type: monitorType, ID: monitorID}},
		AutoIncident:    true,
	}
}

func makeAlertEvent(severity string, isRecover bool) alert.Event {
	return alert.Event{
		Source:     alert.SourceEndpoint,
		AlertType:  "http_check",
		Severity:   severity,
		IsRecover:  isRecover,
		Message:    "connection refused",
		EntityType: "endpoint",
		EntityID:   "ep-5",
		EntityName: "API Gateway",
		Timestamp:  time.Now(),
	}
}

func TestService_HandleAlertEvent_CreatesAutoIncident(t *testing.T) {
	comp := makeExplicitComponent("endpoint", "ep-5")
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	is := &mockIncidentStore{createIncidentID: "inc-99"}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusMajorOutage
	})

	evt := makeAlertEvent("critical", false)
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	require.Len(t, is.createIncidentCalls, 1, "expected exactly one incident to be created")
	call := is.createIncidentCalls[0]
	assert.Equal(t, status.SeverityCritical, call.incident.Severity)
	assert.Equal(t, status.IncidentInvestigating, call.incident.Status)
	assert.Contains(t, call.incident.Title, comp.DisplayName)
	assert.Equal(t, []string{comp.ID}, call.componentIDs)
	assert.Equal(t, evt.Message, call.initialMessage)
}

func TestService_HandleAlertEvent_SeverityMapping(t *testing.T) {
	cases := []struct {
		alertSeverity    string
		expectedSeverity string
	}{
		{"critical", status.SeverityCritical},
		{"warning", status.SeverityMajor},
		{"info", status.SeverityMinor},
		{"", status.SeverityMinor},
	}
	for _, tc := range cases {
		t.Run(tc.alertSeverity, func(t *testing.T) {
			comp := makeExplicitComponent("endpoint", "ep-5")
			cs := &mockComponentStore{}
			cs.setComponentsByMonitor([]status.Component{*comp})

			is := &mockIncidentStore{createIncidentID: "inc-1"}
			svc := newTestService(cs, is)
			svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
				return status.StatusMajorOutage
			})

			evt := makeAlertEvent(tc.alertSeverity, false)
			svc.HandleAlertEvent(context.Background(), evt)

			is.mu.Lock()
			defer is.mu.Unlock()
			require.Len(t, is.createIncidentCalls, 1)
			assert.Equal(t, tc.expectedSeverity, is.createIncidentCalls[0].incident.Severity)
		})
	}
}

func TestService_HandleAlertEvent_ResolvesExistingIncident(t *testing.T) {
	comp := makeExplicitComponent("endpoint", "ep-5")
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	existing := &status.Incident{ID: "inc-77", Title: "API Gateway - connection refused", Status: status.IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[string]*status.Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)
	// Monitor is now operational (recovery).
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusOperational
	})

	evt := makeAlertEvent("critical", true)
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	require.Len(t, is.createUpdateCalls, 1, "expected one update to be created for resolution")
	upd := is.createUpdateCalls[0]
	assert.Equal(t, existing.ID, upd.IncidentID)
	assert.Equal(t, status.IncidentResolved, upd.Status)
	assert.True(t, upd.IsAuto)

	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_UpdatesExistingIncidentOnRepeat(t *testing.T) {
	comp := makeExplicitComponent("endpoint", "ep-5")
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	existing := &status.Incident{ID: "inc-55", Title: "API Gateway - first alert", Status: status.IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[string]*status.Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusMajorOutage
	})

	evt := makeAlertEvent("warning", false)
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	assert.Empty(t, is.createIncidentCalls, "no new incident should be created for a repeat fire")
	require.Len(t, is.createUpdateCalls, 1)
	upd := is.createUpdateCalls[0]
	assert.Equal(t, existing.ID, upd.IncidentID)
	assert.Equal(t, existing.Status, upd.Status)
	assert.True(t, upd.IsAuto)
	assert.Equal(t, evt.Message, upd.Message)
}

func TestService_HandleAlertEvent_SkipsWhenNoIncidentStore(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)

	assert.NotPanics(t, func() {
		svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))
	})
}

func TestService_HandleAlertEvent_SkipsWhenComponentNotFound(t *testing.T) {
	cs := &mockComponentStore{} // returns empty slice
	is := &mockIncidentStore{}
	svc := newTestService(cs, is)

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
	assert.Empty(t, is.createUpdateCalls)
}

func TestService_HandleAlertEvent_SkipsWhenComponentNotAutoIncident(t *testing.T) {
	comp := &status.Component{
		ID:              "comp-10",
		DisplayName:     "API Gateway",
		CompositionMode: status.CompositionExplicit,
		Monitors:        []status.MonitorRef{{Type: "endpoint", ID: "ep-5"}},
		AutoIncident:    false,
	}
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	is := &mockIncidentStore{}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusMajorOutage
	})

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_SkipsWhenComponentStoreLookupFails(t *testing.T) {
	cs := &mockComponentStore{
		componentsByMonitorErr: fmt.Errorf("db connection lost"),
	}
	is := &mockIncidentStore{}
	svc := newTestService(cs, is)

	assert.NotPanics(t, func() {
		svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))
	})

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_RecoverWithNoActiveIncidentIsNoop(t *testing.T) {
	comp := makeExplicitComponent("endpoint", "ep-5")
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp})

	is := &mockIncidentStore{}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusOperational
	})

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", true))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
	assert.Empty(t, is.createUpdateCalls)
}

func TestService_HandleAlertEvent_MultiComponentBroadcast(t *testing.T) {
	comp1 := &status.Component{
		ID:              "comp-10",
		DisplayName:     "API Gateway",
		CompositionMode: status.CompositionExplicit,
		Monitors:        []status.MonitorRef{{Type: "endpoint", ID: "ep-5"}},
		AutoIncident:    true,
	}
	comp2 := &status.Component{
		ID:              "comp-20",
		DisplayName:     "Frontend",
		CompositionMode: status.CompositionExplicit,
		Monitors:        []status.MonitorRef{{Type: "endpoint", ID: "ep-5"}},
		AutoIncident:    true,
	}
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]status.Component{*comp1, *comp2})

	is := &mockIncidentStore{createIncidentID: "inc-1"}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return status.StatusMajorOutage
	})

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))

	is.mu.Lock()
	defer is.mu.Unlock()
	// Both components should have incidents created.
	assert.Len(t, is.createIncidentCalls, 2)
}
