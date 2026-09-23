// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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

func (m *mockComponentStore) ListVisibleComponents(ctx context.Context) ([]Component, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.visibleComponents, m.visibleErr
}

func (m *mockComponentStore) ListComponentsByMonitor(ctx context.Context, monitorType string, monitorID string) ([]Component, error) {
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
func (m *mockComponentStore) ListComponents(ctx context.Context) ([]Component, error) {
	return nil, nil
}
func (m *mockComponentStore) GetComponent(ctx context.Context, id string) (*Component, error) {
	return nil, nil
}
func (m *mockComponentStore) CreateComponent(ctx context.Context, c *Component) (string, error) {
	return "", nil
}
func (m *mockComponentStore) UpdateComponent(ctx context.Context, c *Component) error { return nil }
func (m *mockComponentStore) DeleteComponent(ctx context.Context, id string) error    { return nil }

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

	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return StatusDegraded
	})

	override := StatusMajorOutage
	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: "1"}},
		StatusOverride:  &override,
	}

	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusMajorOutage, got)
}

func TestService_DeriveComponentStatus_ExplicitSingleMonitor(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, monitorType string, monitorID string) string {
		if monitorType == "endpoint" && monitorID == "42" {
			return StatusPartialOutage
		}
		return StatusOperational
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: "42"}},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusPartialOutage, got)
}

func TestService_DeriveComponentStatus_ExplicitMultiMonitor(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id string) string {
		if id == "1" {
			return StatusMajorOutage
		}
		return StatusOperational
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors: []MonitorRef{
			{Type: "endpoint", ID: "1"},
			{Type: "endpoint", ID: "2"},
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
			{Type: "container", ID: "1"},
			{Type: "container", ID: "2"},
		}
	})
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id string) string {
		if id == "1" {
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
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return StatusMajorOutage
	})

	override := StatusUnderMaint
	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: "1"}},
		StatusOverride:  &override,
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusUnderMaint, got)
}

func TestService_DeriveComponentStatus_EmptyProviderResultDefaultsToOperational(t *testing.T) {
	cs := &mockComponentStore{}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return "" // provider returns empty — treated as operational by aggregate
	})

	c := &Component{
		CompositionMode: CompositionExplicit,
		Monitors:        []MonitorRef{{Type: "endpoint", ID: "7"}},
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
		Monitors:        []MonitorRef{{Type: "heartbeat", ID: "3"}},
	}
	got := svc.DeriveComponentStatus(context.Background(), c)
	assert.Equal(t, StatusOperational, got)
}

// --- ComputeGlobalStatus ---

func TestService_ComputeGlobalStatus_AllOperational(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: "1", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "1"}}},
			{ID: "2", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "2"}}},
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
			{ID: "1", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "1"}}},
			{ID: "2", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "2"}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id string) string {
		if id == "2" {
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
			{ID: "1", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "1"}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, _ string) string {
		return StatusPartialOutage
	})

	st, msg := svc.ComputeGlobalStatus(context.Background())
	assert.Equal(t, StatusPartialOutage, st)
	assert.Equal(t, GlobalPartialOutage, msg)
}

func TestService_ComputeGlobalStatus_OneMajorOutage(t *testing.T) {
	cs := &mockComponentStore{
		visibleComponents: []Component{
			{ID: "1", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "1"}}},
			{ID: "2", CompositionMode: CompositionExplicit, Monitors: []MonitorRef{{Type: "endpoint", ID: "2"}}},
		},
	}
	svc := newTestService(cs, nil)
	svc.SetMonitorStatusProvider(func(_ context.Context, _ string, id string) string {
		if id == "1" {
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
			{ID: "1", CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusDegraded)},
			{ID: "2", CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusPartialOutage)},
			{ID: "3", CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusMajorOutage)},
			{ID: "4", CompositionMode: CompositionExplicit, StatusOverride: strPtr(StatusUnderMaint)},
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
