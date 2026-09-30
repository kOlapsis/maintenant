// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCEServices() *Services {
	return &Services{
		Logger:  slog.Default(),
		Version: "test",
	}
}

// withEdition overrides the global edition for the duration of a test.
func withEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = original })
}

// --- fakes ---

type mcpAlertStore struct {
	alerts map[string]*alert.Alert
}

func newMCPAlertStore() *mcpAlertStore {
	return &mcpAlertStore{alerts: map[string]*alert.Alert{}}
}

func (m *mcpAlertStore) InsertAlert(_ context.Context, a *alert.Alert) (string, error) {
	m.alerts[a.ID] = a
	return a.ID, nil
}
func (m *mcpAlertStore) GetAlert(_ context.Context, id string) (*alert.Alert, error) {
	return m.alerts[id], nil
}
func (m *mcpAlertStore) ListAlerts(_ context.Context, _ alert.ListAlertsOpts) ([]*alert.Alert, error) {
	return nil, nil
}
func (m *mcpAlertStore) UpdateAlertStatus(_ context.Context, _ string, _ string, _ *time.Time, _ *string) error {
	return nil
}
func (m *mcpAlertStore) UpdateAlertOnEscalation(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (m *mcpAlertStore) GetActiveAlert(_ context.Context, _, _, _, _ string) (*alert.Alert, error) {
	return nil, nil
}
func (m *mcpAlertStore) ListActiveAlerts(_ context.Context) ([]*alert.Alert, error) { return nil, nil }
func (m *mcpAlertStore) DeleteInactiveAlertsOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mcpAlertStore) AcknowledgeAlert(_ context.Context, _ string, _ string, _ time.Time) (bool, error) {
	return false, nil
}
func (m *mcpAlertStore) SetEscalatedAt(_ context.Context, _ string, _ time.Time) error { return nil }
func (m *mcpAlertStore) ListUnacknowledgedActiveAlerts(_ context.Context) ([]*alert.Alert, error) {
	return nil, nil
}

type mcpIncidentStore struct {
	incidents map[string]*status.Incident
	updates   []*status.IncidentUpdate
	nextID    int
}

func newMCPIncidentStore() *mcpIncidentStore {
	return &mcpIncidentStore{incidents: map[string]*status.Incident{}}
}

func (m *mcpIncidentStore) ListIncidents(_ context.Context, _ status.ListIncidentsOpts) ([]status.Incident, int, error) {
	return nil, 0, nil
}
func (m *mcpIncidentStore) ListActiveIncidents(_ context.Context) ([]status.Incident, error) {
	return nil, nil
}
func (m *mcpIncidentStore) ListRecentIncidents(_ context.Context, _ int) ([]status.Incident, error) {
	return nil, nil
}
func (m *mcpIncidentStore) GetIncident(_ context.Context, id string) (*status.Incident, error) {
	return m.incidents[id], nil
}
func (m *mcpIncidentStore) GetActiveIncidentByComponent(_ context.Context, _ string) (*status.Incident, error) {
	return nil, nil
}
func (m *mcpIncidentStore) CreateIncident(_ context.Context, inc *status.Incident, _ []string, _ string) (string, error) {
	m.nextID++
	inc.ID = "inc-" + strconv.Itoa(m.nextID)
	cp := *inc
	m.incidents[inc.ID] = &cp
	return inc.ID, nil
}
func (m *mcpIncidentStore) UpdateIncident(_ context.Context, _ *status.Incident, _ []string) error {
	return nil
}
func (m *mcpIncidentStore) DeleteIncident(_ context.Context, _ string) error { return nil }
func (m *mcpIncidentStore) ListUpdates(_ context.Context, _ string) ([]status.IncidentUpdate, error) {
	return nil, nil
}
func (m *mcpIncidentStore) CreateUpdate(_ context.Context, u *status.IncidentUpdate) (string, error) {
	m.updates = append(m.updates, u)
	return "upd-1", nil
}

type mcpMaintenanceStore struct {
	windows map[string]*status.MaintenanceWindow
	nextID  int
}

func newMCPMaintenanceStore() *mcpMaintenanceStore {
	return &mcpMaintenanceStore{windows: map[string]*status.MaintenanceWindow{}}
}

func (m *mcpMaintenanceStore) ListMaintenance(_ context.Context, _ string, _ int) ([]status.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mcpMaintenanceStore) GetMaintenance(_ context.Context, id string) (*status.MaintenanceWindow, error) {
	return m.windows[id], nil
}
func (m *mcpMaintenanceStore) CreateMaintenance(_ context.Context, mw *status.MaintenanceWindow, _ []string) (string, error) {
	m.nextID++
	mw.ID = "mw-" + strconv.Itoa(m.nextID)
	cp := *mw
	m.windows[mw.ID] = &cp
	return mw.ID, nil
}
func (m *mcpMaintenanceStore) UpdateMaintenance(_ context.Context, _ *status.MaintenanceWindow, _ []string) error {
	return nil
}
func (m *mcpMaintenanceStore) DeleteMaintenance(_ context.Context, _ string) error { return nil }
func (m *mcpMaintenanceStore) GetPendingActivation(_ context.Context, _ int64) ([]status.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mcpMaintenanceStore) GetPendingDeactivation(_ context.Context, _ int64) ([]status.MaintenanceWindow, error) {
	return nil, nil
}
func (m *mcpMaintenanceStore) SetActive(_ context.Context, _ string, _ bool, _ *string) error {
	return nil
}
func (m *mcpMaintenanceStore) CoveredByAnotherActiveWindow(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

// --- create_incident ---

func TestCreateIncidentHandler_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Incidents: newMCPIncidentStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := createIncidentHandler(svc)(context.Background(), nil, createIncidentInput{Title: "x", Severity: "major"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestCreateIncidentHandler_Pro_Success(t *testing.T) {
	withEdition(t, extension.Pro)
	store := newMCPIncidentStore()
	svc := &Services{Incidents: store, Logger: slog.Default(), Version: "test"}

	result, _, err := createIncidentHandler(svc)(context.Background(), nil, createIncidentInput{
		Title:    "API down",
		Severity: "critical",
		Message:  "investigating",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.Len(t, store.incidents, 1)
	for _, inc := range store.incidents {
		assert.Equal(t, "API down", inc.Title)
		assert.Equal(t, status.IncidentInvestigating, inc.Status)
	}
}

func TestCreateIncidentHandler_Pro_MissingSeverity(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{Incidents: newMCPIncidentStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := createIncidentHandler(svc)(context.Background(), nil, createIncidentInput{Title: "x"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "invalid input")
}

func TestCreateIncidentHandler_Pro_RefusesValuesOutsideTheModel(t *testing.T) {
	withEdition(t, extension.Pro)
	for _, in := range []createIncidentInput{
		{Title: "x", Severity: "apocalyptic"},
		{Title: "x", Severity: "minor", Status: "panicking"},
	} {
		store := newMCPIncidentStore()
		svc := &Services{Incidents: store, Logger: slog.Default(), Version: "test"}

		result, _, err := createIncidentHandler(svc)(context.Background(), nil, in)
		require.NoError(t, err)
		assert.True(t, result.IsError, "%+v", in)
		assert.Contains(t, textFromContent(t, result.Content), "must be one of")
		assert.Empty(t, store.incidents)
	}
}

func TestUpdateIncidentHandler_Pro_RefusesAStatusOutsideTheModel(t *testing.T) {
	withEdition(t, extension.Pro)
	store := newMCPIncidentStore()
	store.incidents["inc-1"] = &status.Incident{ID: "inc-1", Title: "API down", Status: status.IncidentInvestigating}
	svc := &Services{Incidents: store, Logger: slog.Default(), Version: "test"}

	result, _, err := updateIncidentHandler(svc)(context.Background(), nil, updateIncidentInput{IncidentID: "inc-1", Status: "panicking", Message: "hm"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "status must be one of")
	assert.Empty(t, store.updates)
}

type recordingAnnouncer struct {
	opened  []string
	updates []string
}

func (a *recordingAnnouncer) AnnounceIncident(_ context.Context, inc *status.Incident, message string) {
	a.opened = append(a.opened, inc.Title+"|"+message)
}

func (a *recordingAnnouncer) AnnounceIncidentUpdate(_ context.Context, inc *status.Incident, upd *status.IncidentUpdate) {
	a.updates = append(a.updates, inc.Status+">"+upd.Status+"|"+upd.Message)
}

func TestCreateIncidentHandler_AnnouncesTheIncident(t *testing.T) {
	withEdition(t, extension.Pro)
	announcer := &recordingAnnouncer{}
	svc := &Services{Incidents: newMCPIncidentStore(), IncidentAnnouncer: announcer, Logger: slog.Default(), Version: "test"}

	result, _, err := createIncidentHandler(svc)(context.Background(), nil, createIncidentInput{
		Title:    "API down",
		Severity: "critical",
		Message:  "investigating",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, []string{"API down|investigating"}, announcer.opened)
}

func TestUpdateIncidentHandler_AnnouncesTheUpdateAgainstThePreviousState(t *testing.T) {
	withEdition(t, extension.Pro)
	store := newMCPIncidentStore()
	store.incidents["inc-1"] = &status.Incident{ID: "inc-1", Title: "API down", Status: status.IncidentInvestigating}
	announcer := &recordingAnnouncer{}
	svc := &Services{Incidents: store, IncidentAnnouncer: announcer, Logger: slog.Default(), Version: "test"}

	result, _, err := updateIncidentHandler(svc)(context.Background(), nil, updateIncidentInput{IncidentID: "inc-1", Status: "resolved", Message: "fixed"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, []string{"investigating>resolved|fixed"}, announcer.updates)
}

// --- update_incident ---

func TestUpdateIncidentHandler_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Incidents: newMCPIncidentStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := updateIncidentHandler(svc)(context.Background(), nil, updateIncidentInput{IncidentID: "inc-1", Status: "resolved", Message: "fixed"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestUpdateIncidentHandler_Pro_Success(t *testing.T) {
	withEdition(t, extension.Pro)
	store := newMCPIncidentStore()
	store.incidents["inc-1"] = &status.Incident{ID: "inc-1", Title: "API down", Status: status.IncidentInvestigating}
	svc := &Services{Incidents: store, Logger: slog.Default(), Version: "test"}

	result, _, err := updateIncidentHandler(svc)(context.Background(), nil, updateIncidentInput{IncidentID: "inc-1", Status: "resolved", Message: "fixed"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, store.updates, 1)
	assert.Equal(t, "resolved", store.updates[0].Status)
	assert.Equal(t, "fixed", store.updates[0].Message)
}

func TestUpdateIncidentHandler_Pro_NotFound(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{Incidents: newMCPIncidentStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := updateIncidentHandler(svc)(context.Background(), nil, updateIncidentInput{IncidentID: "missing", Status: "resolved", Message: "fixed"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not found")
}

// --- create_maintenance ---

func TestCreateMaintenanceHandler_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Maintenance: newMCPMaintenanceStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := createMaintenanceHandler(svc)(context.Background(), nil, createMaintenanceInput{
		Title:     "Scheduled update",
		StartTime: "2026-03-01T02:00:00Z",
		EndTime:   "2026-03-01T04:00:00Z",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestCreateMaintenanceHandler_Pro_Success(t *testing.T) {
	withEdition(t, extension.Pro)
	store := newMCPMaintenanceStore()
	svc := &Services{Maintenance: store, Logger: slog.Default(), Version: "test"}

	result, _, err := createMaintenanceHandler(svc)(context.Background(), nil, createMaintenanceInput{
		Title:     "Scheduled update",
		StartTime: "2026-03-01T02:00:00Z",
		EndTime:   "2026-03-01T04:00:00Z",
		Message:   "Updating servers",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, store.windows, 1)
	for _, mw := range store.windows {
		assert.Equal(t, "Scheduled update", mw.Title)
		assert.Equal(t, "Updating servers", mw.Description)
	}
}

func TestCreateMaintenanceHandler_Pro_InvalidWindow(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{Maintenance: newMCPMaintenanceStore(), Logger: slog.Default(), Version: "test"}

	result, _, err := createMaintenanceHandler(svc)(context.Background(), nil, createMaintenanceInput{
		Title:     "Scheduled update",
		StartTime: "2026-03-01T04:00:00Z",
		EndTime:   "2026-03-01T02:00:00Z", // end before start
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "invalid input")
}

// --- monitors ---

func TestPauseMonitorHandler_InvalidType(t *testing.T) {
	svc := newCEServices()
	handler := pauseMonitorHandler(svc)

	input := pauseMonitorInput{
		MonitorType: "endpoint",
		MonitorID:   "1",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestPauseMonitorHandler_EmptyType(t *testing.T) {
	svc := newCEServices()
	handler := pauseMonitorHandler(svc)

	input := pauseMonitorInput{
		MonitorType: "",
		MonitorID:   "1",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestResumeMonitorHandler_InvalidType(t *testing.T) {
	svc := newCEServices()
	handler := resumeMonitorHandler(svc)

	input := resumeMonitorInput{
		MonitorType: "container",
		MonitorID:   "1",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestResumeMonitorHandler_EmptyType(t *testing.T) {
	svc := newCEServices()
	handler := resumeMonitorHandler(svc)

	input := resumeMonitorInput{
		MonitorType: "",
		MonitorID:   "1",
	}
	result, _, err := handler(context.Background(), nil, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, "invalid input: only 'heartbeat' monitor type is supported", textFromContent(t, result.Content))
}

func TestWriteToolRegistration(t *testing.T) {
	svc := newCEServices()
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "maintenant-test",
		Version: "0.0.1",
	}, nil)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("registerWriteTools panics due to go-sdk v1.4.0 jsonschema tag parsing: %v", r)
			}
		}()
		registerWriteTools(server, svc)
	}()
}
