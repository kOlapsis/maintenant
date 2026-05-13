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

package status

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock stores ---

// mockComponentStore implements ComponentStore. Only the methods exercised by
// the tested code paths are given real behaviour; all others return zero values.
type mockComponentStore struct {
	mu                     sync.Mutex
	visibleComponents      []Component
	visibleErr             error
	componentsByMonitor    []Component
	componentsByMonitorErr error
	removeDanglingCalls    []string // track calls: "type:id"
}

func (m *mockComponentStore) setComponentsByMonitor(comps []Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.componentsByMonitor = comps
}

func (m *mockComponentStore) ListVisibleComponents(ctx context.Context) ([]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.visibleComponents, m.visibleErr
}

func (m *mockComponentStore) ListComponentsByMonitor(ctx context.Context, monitorType string, monitorID int64) ([]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.componentsByMonitorErr != nil {
		return nil, m.componentsByMonitorErr
	}
	return m.componentsByMonitor, nil
}

func (m *mockComponentStore) RemoveDanglingMonitorRefs(ctx context.Context, monitorType string, monitorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeDanglingCalls = append(m.removeDanglingCalls, fmt.Sprintf("%s:%d", monitorType, monitorID))
	return nil
}

// Unused methods — satisfy interface with zero values.
func (m *mockComponentStore) ListComponents(ctx context.Context) ([]Component, error) {
	return nil, nil
}
func (m *mockComponentStore) GetComponent(ctx context.Context, id int64) (*Component, error) {
	return nil, nil
}
func (m *mockComponentStore) CreateComponent(ctx context.Context, c *Component) (int64, error) {
	return 0, nil
}
func (m *mockComponentStore) UpdateComponent(ctx context.Context, c *Component) error { return nil }
func (m *mockComponentStore) DeleteComponent(ctx context.Context, id int64) error     { return nil }

// mockIncidentStore implements IncidentStore. Call counts and arguments are
// captured so tests can assert what was called.
type mockIncidentStore struct {
	mu                   sync.Mutex
	activeByComponent    map[int64]*Incident
	activeByComponentErr error
	createIncidentID     int64
	createIncidentErr    error
	createIncidentCalls  []createIncidentCall
	createUpdateID       int64
	createUpdateErr      error
	createUpdateCalls    []IncidentUpdate
	listActiveIncidents  []Incident
	listActiveErr        error
	listRecentIncidents  []Incident
	listRecentErr        error
}

type createIncidentCall struct {
	incident       Incident
	componentIDs   []int64
	initialMessage string
}

func (m *mockIncidentStore) GetActiveIncidentByComponent(ctx context.Context, componentID int64) (*Incident, error) {
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

func (m *mockIncidentStore) CreateIncident(ctx context.Context, inc *Incident, componentIDs []int64, initialMessage string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createIncidentErr != nil {
		return 0, m.createIncidentErr
	}
	m.createIncidentCalls = append(m.createIncidentCalls, createIncidentCall{
		incident:       *inc,
		componentIDs:   componentIDs,
		initialMessage: initialMessage,
	})
	return m.createIncidentID, nil
}

func (m *mockIncidentStore) CreateUpdate(ctx context.Context, u *IncidentUpdate) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createUpdateErr != nil {
		return 0, m.createUpdateErr
	}
	m.createUpdateCalls = append(m.createUpdateCalls, *u)
	return m.createUpdateID, nil
}

func (m *mockIncidentStore) ListActiveIncidents(ctx context.Context) ([]Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listActiveIncidents, m.listActiveErr
}

func (m *mockIncidentStore) ListRecentIncidents(ctx context.Context, days int) ([]Incident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listRecentIncidents, m.listRecentErr
}

// Unused methods.
func (m *mockIncidentStore) ListIncidents(ctx context.Context, opts ListIncidentsOpts) ([]Incident, int, error) {
	return nil, 0, nil
}
func (m *mockIncidentStore) GetIncident(ctx context.Context, id int64) (*Incident, error) {
	return nil, nil
}
func (m *mockIncidentStore) UpdateIncident(ctx context.Context, inc *Incident, componentIDs []int64) error {
	return nil
}
func (m *mockIncidentStore) DeleteIncident(ctx context.Context, id int64) error { return nil }
func (m *mockIncidentStore) ListUpdates(ctx context.Context, incidentID int64) ([]IncidentUpdate, error) {
	return nil, nil
}
func (m *mockIncidentStore) DeleteIncidentsOlderThan(ctx context.Context, days int) (int64, error) {
	return 0, nil
}

// --- Helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 10}))
}

func newTestService(cs ComponentStore, is IncidentStore) *Service {
	return NewService(Deps{
		Components: cs,
		Logger:     discardLogger(),
		Incidents:  is,
	})
}

func strPtr(s string) *string { return &s }

// makeExplicitComponent creates a component with explicit composition mode and one monitor.
func makeExplicitComponent(monitorType string, monitorID int64) *Component {
	return &Component{
		ID:              10,
		DisplayName:     "API Gateway",
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: monitorType, ID: monitorID}},
		AutoIncident:    true,
	}
}

// --- ComputeAggregateStatus ---

func TestComputeAggregateStatus_Empty(t *testing.T) {
	assert.Equal(t, StatusOperational, ComputeAggregateStatus(nil))
	assert.Equal(t, StatusOperational, ComputeAggregateStatus([]string{}))
}

func TestComputeAggregateStatus_AllOperational(t *testing.T) {
	assert.Equal(t, StatusOperational, ComputeAggregateStatus([]string{StatusOperational, StatusOperational}))
}

func TestComputeAggregateStatus_AllMajor(t *testing.T) {
	assert.Equal(t, StatusMajorOutage, ComputeAggregateStatus([]string{StatusMajorOutage, StatusMajorOutage}))
}

func TestComputeAggregateStatus_MixMajorOp(t *testing.T) {
	assert.Equal(t, StatusPartialOutage, ComputeAggregateStatus([]string{StatusMajorOutage, StatusOperational}))
}

func TestComputeAggregateStatus_DegradedAndOperational(t *testing.T) {
	assert.Equal(t, StatusDegraded, ComputeAggregateStatus([]string{StatusDegraded, StatusOperational}))
}

func TestComputeAggregateStatus_OnlyDegraded(t *testing.T) {
	assert.Equal(t, StatusDegraded, ComputeAggregateStatus([]string{StatusDegraded, StatusDegraded}))
}

func TestComputeAggregateStatus_MajorAndDegraded(t *testing.T) {
	// major + degraded (no operational) → partial (major dominates but not all major)
	assert.Equal(t, StatusPartialOutage, ComputeAggregateStatus([]string{StatusMajorOutage, StatusDegraded}))
}

// --- DeriveComponentStatus ---

func TestService_DeriveComponentStatus_OverrideTakesPrecedence(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)

	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusDegraded
	})

	override := StatusMajorOutage
	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 1}},
		StatusOverride:  &override,
	}

	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusMajorOutage, got)
}

func TestService_DeriveComponentStatus_ExplicitSingleMonitor(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, monitorType string, monitorID int64) string {
		if monitorType == "endpoint" && monitorID == 42 {
			return StatusPartialOutage
		}
		return StatusOperational
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 42}},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusPartialOutage, got)
}

func TestService_DeriveComponentStatus_ExplicitMultiMonitor(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 1 {
			return StatusMajorOutage
		}
		return StatusOperational
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors: []MonitorRef{
			{Type: "endpoint", ID: 1},
			{Type: "endpoint", ID: 2},
		},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	// one major + one operational → partial_outage
	assert.Equal(t, StatusPartialOutage, got)
}

func TestService_DeriveComponentStatus_ExplicitNoMonitors_NeedsAttention(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
	assert.True(t, c.NeedsAttention)
}

func TestService_DeriveComponentStatus_MatchAllEmpty(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorPopulationProvider(func(_ context.Context, _ string) []MonitorRef {
		return nil // no monitors of this type
	})

	c := &Component{
		CompositionMode: CompositionMatchAll,
		MatchAllType:    "container",
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
}

func TestService_DeriveComponentStatus_MatchAllAggregates(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorPopulationProvider(func(_ context.Context, _ string) []MonitorRef {
		return []MonitorRef{
			{Type: "container", ID: 1},
			{Type: "container", ID: 2},
		}
	})
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 1 {
			return StatusMajorOutage
		}
		return StatusOperational
	})

	c := &Component{
		CompositionMode: CompositionMatchAll,
		MatchAllType:    "container",
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusPartialOutage, got)
}

func TestService_DeriveComponentStatus_OverrideBlocksAggregate(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusMajorOutage
	})

	override := StatusUnderMaint
	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 1}},
		StatusOverride:  &override,
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusUnderMaint, got)
}

func TestService_DeriveComponentStatus_EmptyProviderResultDefaultsToOperational(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return "" // provider returns empty — treated as operational by aggregate
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 7}},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	// empty string is not StatusMajorOutage/Degraded/Partial → treated as operational
	assert.Equal(t, StatusOperational, got)
}

func TestService_DeriveComponentStatus_NoProviderDefaultsToOperational(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "heartbeat", ID: 3}},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
}

// --- ComputeGlobalStatus ---

func TestService_ComputeGlobalStatus_AllOperational(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 1}}},
			{ID: 2, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 2}}},
		},
	}
	svc := newTestService(cs, nil)

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusOperational, st)
	assert.Equal(t, GlobalAllOperational, msg)
}

func TestService_ComputeGlobalStatus_OneDegraded(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 1}}},
			{ID: 2, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 2}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 2 {
			return StatusDegraded
		}
		return StatusOperational
	})

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusDegraded, st)
	assert.Equal(t, GlobalDegraded, msg)
}

func TestService_ComputeGlobalStatus_OnePartialOutage(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 1}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusPartialOutage
	})

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusPartialOutage, st)
	assert.Equal(t, GlobalPartialOutage, msg)
}

func TestService_ComputeGlobalStatus_OneMajorOutage(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 1}}},
			{ID: 2, CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: 2}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 1 {
			return StatusMajorOutage
		}
		return StatusOperational
	})

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusMajorOutage, st)
	assert.Equal(t, GlobalMajorOutage, msg)
}

func TestService_ComputeGlobalStatus_WorstWins(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusDegraded)},
			{ID: 2, CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusPartialOutage)},
			{ID: 3, CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusMajorOutage)},
			{ID: 4, CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusUnderMaint)},
		},
	}
	svc := newTestService(cs, nil)

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusMajorOutage, st)
	assert.Equal(t, GlobalMajorOutage, msg)
}

func TestService_ComputeGlobalStatus_NoComponents(t *testing.T) {
	cs := &mockComponentStore{visibleComponents: []Component{}}
	svc := newTestService(cs, nil)

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusOperational, st)
	assert.Equal(t, GlobalAllOperational, msg)
}

// --- statusSeverity / Severity ---

func TestStatusSeverity_Values(t *testing.T) {
	cases := []struct {
		status   string
		expected int
	}{
		{StatusMajorOutage, 4},
		{StatusUnderMaint, 3},
		{StatusPartialOutage, 2},
		{StatusDegraded, 1},
		{StatusOperational, 0},
		{"unknown_value", 0},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			assert.Equal(t, tc.expected, statusSeverity(tc.status))
			assert.Equal(t, tc.expected, Severity(tc.status), "exported Severity must match")
		})
	}
}

// --- statusLabel ---

func TestStatusLabel_AllStatuses(t *testing.T) {
	cases := []struct {
		status   string
		expected string
	}{
		{StatusOperational, "Operational"},
		{StatusDegraded, "Degraded Performance"},
		{StatusPartialOutage, "Partial Outage"},
		{StatusMajorOutage, "Major Outage"},
		{StatusUnderMaint, "Under Maintenance"},
		{"anything_else", "Unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			assert.Equal(t, tc.expected, statusLabel(tc.status))
		})
	}
}

// --- HandleAlertEvent ---

func makeAlertEvent(severity string, isRecover bool) alert.Event {
	return alert.Event{
		Source:     alert.SourceEndpoint,
		AlertType:  "http_check",
		Severity:   severity,
		IsRecover:  isRecover,
		Message:    "connection refused",
		EntityType: "endpoint",
		EntityID:   5,
		EntityName: "API Gateway",
		Timestamp:  time.Now(),
	}
}

func TestService_HandleAlertEvent_CreatesAutoIncident(t *testing.T) {
	comp := makeExplicitComponent("endpoint", 5)
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp})

	is := &mockIncidentStore{createIncidentID: 99}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusMajorOutage
	})

	evt := makeAlertEvent("critical", false)
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	require.Len(t, is.createIncidentCalls, 1, "expected exactly one incident to be created")
	call := is.createIncidentCalls[0]
	assert.Equal(t, SeverityCritical, call.incident.Severity)
	assert.Equal(t, IncidentInvestigating, call.incident.Status)
	assert.Contains(t, call.incident.Title, comp.DisplayName)
	assert.Equal(t, []int64{comp.ID}, call.componentIDs)
	assert.Equal(t, evt.Message, call.initialMessage)
}

func TestService_HandleAlertEvent_SeverityMapping(t *testing.T) {
	cases := []struct {
		alertSeverity    string
		expectedSeverity string
	}{
		{"critical", SeverityCritical},
		{"warning", SeverityMajor},
		{"info", SeverityMinor},
		{"", SeverityMinor},
	}
	for _, tc := range cases {
		t.Run(tc.alertSeverity, func(t *testing.T) {
			comp := makeExplicitComponent("endpoint", 5)
			cs := &mockComponentStore{}
			cs.setComponentsByMonitor([]Component{*comp})

			is := &mockIncidentStore{createIncidentID: 1}
			svc := newTestService(cs, is)
			svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
				return StatusMajorOutage
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
	comp := makeExplicitComponent("endpoint", 5)
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp})

	existing := &Incident{ID: 77, Title: "API Gateway - connection refused", Status: IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[int64]*Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)
	// Monitor is now operational (recovery).
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusOperational
	})

	evt := makeAlertEvent("critical", true)
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	require.Len(t, is.createUpdateCalls, 1, "expected one update to be created for resolution")
	upd := is.createUpdateCalls[0]
	assert.Equal(t, existing.ID, upd.IncidentID)
	assert.Equal(t, IncidentResolved, upd.Status)
	assert.True(t, upd.IsAuto)

	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_UpdatesExistingIncidentOnRepeat(t *testing.T) {
	comp := makeExplicitComponent("endpoint", 5)
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp})

	existing := &Incident{ID: 55, Title: "API Gateway - first alert", Status: IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[int64]*Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusMajorOutage
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
	comp := &Component{
		ID:              10,
		DisplayName:     "API Gateway",
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 5}},
		AutoIncident:    false,
	}
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp})

	is := &mockIncidentStore{}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusMajorOutage
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
	comp := makeExplicitComponent("endpoint", 5)
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp})

	is := &mockIncidentStore{}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusOperational
	})

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", true))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
	assert.Empty(t, is.createUpdateCalls)
}

func TestService_HandleAlertEvent_MultiComponentBroadcast(t *testing.T) {
	comp1 := &Component{
		ID:              10,
		DisplayName:     "API Gateway",
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 5}},
		AutoIncident:    true,
	}
	comp2 := &Component{
		ID:              20,
		DisplayName:     "Frontend",
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: 5}},
		AutoIncident:    true,
	}
	cs := &mockComponentStore{}
	cs.setComponentsByMonitor([]Component{*comp1, *comp2})

	is := &mockIncidentStore{createIncidentID: 1}
	svc := newTestService(cs, is)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusMajorOutage
	})

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))

	is.mu.Lock()
	defer is.mu.Unlock()
	// Both components should have incidents created.
	assert.Len(t, is.createIncidentCalls, 2)
}
