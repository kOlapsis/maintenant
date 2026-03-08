// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package status

import (
	"context"
	"errors"
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
	mu                   sync.Mutex
	visibleComponents    []Component
	visibleErr           error
	componentByMonitor   map[string]*Component // key: "type:id"
	componentByMonitorErr error
	globalComponents     []Component
	globalComponentsErr  error
}

func (m *mockComponentStore) setVisibleComponents(comps []Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.visibleComponents = comps
}

func (m *mockComponentStore) setComponentByMonitor(monitorType string, monitorID int64, c *Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.componentByMonitor == nil {
		m.componentByMonitor = make(map[string]*Component)
	}
	key := monitorType + ":" + string(rune(monitorID+'0'))
	m.componentByMonitor[key] = c
}

func (m *mockComponentStore) ListVisibleComponents(ctx context.Context) ([]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.visibleComponents, m.visibleErr
}

func (m *mockComponentStore) GetComponentByMonitor(ctx context.Context, monitorType string, monitorID int64) (*Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.componentByMonitorErr != nil {
		return nil, m.componentByMonitorErr
	}
	if m.componentByMonitor == nil {
		return nil, nil
	}
	key := monitorType + ":" + string(rune(monitorID+'0'))
	return m.componentByMonitor[key], nil
}

func (m *mockComponentStore) ListGlobalComponents(ctx context.Context, monitorType string) ([]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.globalComponents, m.globalComponentsErr
}

// Unused methods — satisfy interface with zero values.
func (m *mockComponentStore) ListGroups(ctx context.Context) ([]ComponentGroup, error) {
	return nil, nil
}
func (m *mockComponentStore) GetGroup(ctx context.Context, id int64) (*ComponentGroup, error) {
	return nil, nil
}
func (m *mockComponentStore) CreateGroup(ctx context.Context, g *ComponentGroup) (int64, error) {
	return 0, nil
}
func (m *mockComponentStore) UpdateGroup(ctx context.Context, g *ComponentGroup) error { return nil }
func (m *mockComponentStore) DeleteGroup(ctx context.Context, id int64) error          { return nil }
func (m *mockComponentStore) ListComponents(ctx context.Context) ([]Component, error)  { return nil, nil }
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
	mu                        sync.Mutex
	activeByComponent         map[int64]*Incident
	activeByComponentErr      error
	createIncidentID          int64
	createIncidentErr         error
	createIncidentCalls       []createIncidentCall
	createUpdateID            int64
	createUpdateErr           error
	createUpdateCalls         []IncidentUpdate
	listActiveIncidents       []Incident
	listActiveErr             error
	listRecentIncidents       []Incident
	listRecentErr             error
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

// --- DeriveComponentStatus ---

func TestService_DeriveComponentStatus_OverrideTakesPrecedence(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)

	// MonitorStatusProvider would return degraded, but override wins.
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusDegraded
	})

	override := StatusMajorOutage
	c := &Component{
		MonitorType:    "endpoint",
		MonitorID:      1,
		StatusOverride: &override,
	}

	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusMajorOutage, got)
}

func TestService_DeriveComponentStatus_UsesMonitorStatusProvider(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, monitorType string, monitorID int64) string {
		if monitorType == "endpoint" && monitorID == 42 {
			return StatusPartialOutage
		}
		return ""
	})

	c := &Component{MonitorType: "endpoint", MonitorID: 42}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusPartialOutage, got)
}

func TestService_DeriveComponentStatus_EmptyProviderResultDefaultsToOperational(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return "" // provider returns empty
	})

	c := &Component{MonitorType: "endpoint", MonitorID: 7}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
}

func TestService_DeriveComponentStatus_NoProviderDefaultsToOperational(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	// No MonitorStatusProvider set.

	c := &Component{MonitorType: "heartbeat", MonitorID: 3}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
}

// --- ComputeGlobalStatus ---

func TestService_ComputeGlobalStatus_AllOperational(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, MonitorType: "endpoint", MonitorID: 1},
			{ID: 2, MonitorType: "endpoint", MonitorID: 2},
		},
	}
	svc := newTestService(cs, nil)
	// No provider — all components default to operational.

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusOperational, status)
	assert.Equal(t, GlobalAllOperational, msg)
}

func TestService_ComputeGlobalStatus_OneDegraded(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, MonitorType: "endpoint", MonitorID: 1},
			{ID: 2, MonitorType: "endpoint", MonitorID: 2},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 2 {
			return StatusDegraded
		}
		return StatusOperational
	})

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusDegraded, status)
	assert.Equal(t, GlobalDegraded, msg)
}

func TestService_ComputeGlobalStatus_OnePartialOutage(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, MonitorType: "endpoint", MonitorID: 1},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ int64) string {
		return StatusPartialOutage
	})

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusPartialOutage, status)
	assert.Equal(t, GlobalPartialOutage, msg)
}

func TestService_ComputeGlobalStatus_OneMajorOutage(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, MonitorType: "endpoint", MonitorID: 1},
			{ID: 2, MonitorType: "endpoint", MonitorID: 2},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id int64) string {
		if id == 1 {
			return StatusMajorOutage
		}
		return StatusOperational
	})

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusMajorOutage, status)
	assert.Equal(t, GlobalMajorOutage, msg)
}

func TestService_ComputeGlobalStatus_WorstWins(t *testing.T) {
	// Mix: degraded, partial, major, maintenance — major_outage wins (severity 4).
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: 1, MonitorType: "endpoint", MonitorID: 1, StatusOverride: strPtr(StatusDegraded)},
			{ID: 2, MonitorType: "endpoint", MonitorID: 2, StatusOverride: strPtr(StatusPartialOutage)},
			{ID: 3, MonitorType: "endpoint", MonitorID: 3, StatusOverride: strPtr(StatusMajorOutage)},
			{ID: 4, MonitorType: "endpoint", MonitorID: 4, StatusOverride: strPtr(StatusUnderMaint)},
		},
	}
	svc := newTestService(cs, nil)

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusMajorOutage, status)
	assert.Equal(t, GlobalMajorOutage, msg)
}

func TestService_ComputeGlobalStatus_NoComponents(t *testing.T) {
	cs := &mockComponentStore{visibleComponents: []Component{}}
	svc := newTestService(cs, nil)

	status, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusOperational, status)
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

func makeAutoIncidentComponent() *Component {
	return &Component{
		ID:           10,
		DisplayName:  "API Gateway",
		MonitorType:  "endpoint",
		MonitorID:    5,
		AutoIncident: true,
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
		EntityID:   5,
		EntityName: "API Gateway",
		Timestamp:  time.Now(),
	}
}

// registerComponentByMonitor is a helper because the mock key format must match
// the exact logic in GetComponentByMonitor.
func registerComponentByMonitor(cs *mockComponentStore, c *Component) {
	if cs.componentByMonitor == nil {
		cs.componentByMonitor = make(map[string]*Component)
	}
	key := c.MonitorType + ":" + string(rune(c.MonitorID+'0'))
	cs.componentByMonitor[key] = c
}

func TestService_HandleAlertEvent_CreatesAutoIncident(t *testing.T) {
	comp := makeAutoIncidentComponent()
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	is := &mockIncidentStore{createIncidentID: 99}
	svc := newTestService(cs, is)

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
			comp := makeAutoIncidentComponent()
			cs := &mockComponentStore{}
			registerComponentByMonitor(cs, comp)

			is := &mockIncidentStore{createIncidentID: 1}
			svc := newTestService(cs, is)

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
	comp := makeAutoIncidentComponent()
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	existing := &Incident{ID: 77, Title: "API Gateway - connection refused", Status: IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[int64]*Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)

	evt := makeAlertEvent("critical", true) // recovery
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	require.Len(t, is.createUpdateCalls, 1, "expected one update to be created for resolution")
	upd := is.createUpdateCalls[0]
	assert.Equal(t, existing.ID, upd.IncidentID)
	assert.Equal(t, IncidentResolved, upd.Status)
	assert.True(t, upd.IsAuto)
	assert.Contains(t, upd.Message, evt.Message)

	// No new incident must have been created.
	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_UpdatesExistingIncidentOnRepeat(t *testing.T) {
	comp := makeAutoIncidentComponent()
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	existing := &Incident{ID: 55, Title: "API Gateway - first alert", Status: IncidentInvestigating}
	is := &mockIncidentStore{
		activeByComponent: map[int64]*Incident{comp.ID: existing},
	}
	svc := newTestService(cs, is)

	evt := makeAlertEvent("warning", false) // fires again, not a recovery
	svc.HandleAlertEvent(context.Background(), evt)

	is.mu.Lock()
	defer is.mu.Unlock()

	// Should add an update to the existing incident, not create a new one.
	assert.Empty(t, is.createIncidentCalls, "no new incident should be created for a repeat fire")
	require.Len(t, is.createUpdateCalls, 1)
	upd := is.createUpdateCalls[0]
	assert.Equal(t, existing.ID, upd.IncidentID)
	assert.Equal(t, existing.Status, upd.Status)
	assert.True(t, upd.IsAuto)
	assert.Equal(t, evt.Message, upd.Message)
}

func TestService_HandleAlertEvent_SkipsWhenNoIncidentStore(t *testing.T) {
	comp := makeAutoIncidentComponent()
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	// No incident store.
	svc := newTestService(cs, nil)

	// Should not panic and should silently skip.
	assert.NotPanics(t, func() {
		svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))
	})
}

func TestService_HandleAlertEvent_SkipsWhenComponentNotFound(t *testing.T) {
	cs := &mockComponentStore{} // no components registered
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
		ID:           10,
		DisplayName:  "API Gateway",
		MonitorType:  "endpoint",
		MonitorID:    5,
		AutoIncident: false, // disabled
	}
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	is := &mockIncidentStore{}
	svc := newTestService(cs, is)

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", false))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
}

func TestService_HandleAlertEvent_SkipsWhenComponentStoreLookupFails(t *testing.T) {
	cs := &mockComponentStore{
		componentByMonitorErr: errors.New("db connection lost"),
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
	comp := makeAutoIncidentComponent()
	cs := &mockComponentStore{}
	registerComponentByMonitor(cs, comp)

	// No active incident for this component.
	is := &mockIncidentStore{}
	svc := newTestService(cs, is)

	svc.HandleAlertEvent(context.Background(), makeAlertEvent("critical", true))

	is.mu.Lock()
	defer is.mu.Unlock()
	assert.Empty(t, is.createIncidentCalls)
	assert.Empty(t, is.createUpdateCalls)
}
