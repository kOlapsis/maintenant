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

package resource

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mockResourceStore — minimal in-memory implementation of ResourceStore.
// ---------------------------------------------------------------------------

type mockResourceStore struct {
	mu           sync.Mutex
	alertConfigs map[int64]*ResourceAlertConfig
	snapshots    []*ResourceSnapshot
}

func newMockResourceStore() *mockResourceStore {
	return &mockResourceStore{
		alertConfigs: make(map[int64]*ResourceAlertConfig),
	}
}

func (m *mockResourceStore) GetAlertConfig(_ context.Context, containerID int64) (*ResourceAlertConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, ok := m.alertConfigs[containerID]
	if !ok {
		return nil, nil
	}
	// Return a shallow copy so mutations in evaluateAlerts don't race with test assertions.
	cp := *cfg
	return &cp, nil
}

func (m *mockResourceStore) UpsertAlertConfig(_ context.Context, cfg *ResourceAlertConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *cfg
	m.alertConfigs[cfg.ContainerID] = &cp
	return nil
}

func (m *mockResourceStore) InsertSnapshot(_ context.Context, s *ResourceSnapshot) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots = append(m.snapshots, s)
	return int64(len(m.snapshots)), nil
}

// Stubbed methods — not exercised by these tests.

func (m *mockResourceStore) GetLatestSnapshot(_ context.Context, _ int64) (*ResourceSnapshot, error) {
	return nil, nil
}
func (m *mockResourceStore) ListSnapshots(_ context.Context, _ int64, _, _ time.Time) ([]*ResourceSnapshot, error) {
	return nil, nil
}
func (m *mockResourceStore) ListSnapshotsAggregated(_ context.Context, _ int64, _, _ time.Time, _ Granularity) ([]*ResourceSnapshot, error) {
	return nil, nil
}
func (m *mockResourceStore) DeleteSnapshotsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockResourceStore) InsertHourlyRollup(_ context.Context, _ *RollupRow) error  { return nil }
func (m *mockResourceStore) InsertDailyRollup(_ context.Context, _ *RollupRow) error   { return nil }
func (m *mockResourceStore) AggregateHourlyRollup(_ context.Context, _, _ time.Time) error {
	return nil
}
func (m *mockResourceStore) AggregateDailyRollup(_ context.Context, _, _ time.Time) error {
	return nil
}
func (m *mockResourceStore) GetTopConsumersByPeriod(_ context.Context, _, _ string, _ int) ([]TopConsumerRow, error) {
	return nil, nil
}
func (m *mockResourceStore) DeleteHourlyBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockResourceStore) DeleteDailyBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// mockContainerStore — minimal ContainerStore used only for GetContainerByID.
// ---------------------------------------------------------------------------

type mockContainerStore struct {
	containers map[int64]*container.Container
}

func newMockContainerStore(containers ...*container.Container) *mockContainerStore {
	s := &mockContainerStore{containers: make(map[int64]*container.Container)}
	for _, c := range containers {
		s.containers[c.ID] = c
	}
	return s
}

func (m *mockContainerStore) GetContainerByID(_ context.Context, id int64) (*container.Container, error) {
	c, ok := m.containers[id]
	if !ok {
		return nil, nil
	}
	return c, nil
}

// All other ContainerStore methods are stubs.

func (m *mockContainerStore) InsertContainer(_ context.Context, _ *container.Container) (int64, error) {
	return 0, nil
}
func (m *mockContainerStore) UpdateContainer(_ context.Context, _ *container.Container) error {
	return nil
}
func (m *mockContainerStore) GetContainerByExternalID(_ context.Context, _ string) (*container.Container, error) {
	return nil, nil
}
func (m *mockContainerStore) ListContainers(_ context.Context, _ container.ListContainersOpts) ([]*container.Container, error) {
	return nil, nil
}
func (m *mockContainerStore) ArchiveContainer(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockContainerStore) DeleteContainerByID(_ context.Context, _ int64) error { return nil }
func (m *mockContainerStore) InsertTransition(_ context.Context, _ *container.StateTransition) (int64, error) {
	return 0, nil
}
func (m *mockContainerStore) ListTransitionsByContainer(_ context.Context, _ int64, _ container.ListTransitionsOpts) ([]*container.StateTransition, int, error) {
	return nil, 0, nil
}
func (m *mockContainerStore) CountRestartsSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return 0, nil
}
func (m *mockContainerStore) GetTransitionsInWindow(_ context.Context, _ int64, _, _ time.Time) ([]*container.StateTransition, error) {
	return nil, nil
}
func (m *mockContainerStore) DeleteTransitionsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (m *mockContainerStore) DeleteArchivedContainersBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// capturedEvent holds one emitted event for assertion.
// ---------------------------------------------------------------------------

type capturedEvent struct {
	eventType string
	data      map[string]interface{}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestService builds a *Service directly (bypassing NewService) so that the
// collector is never started and only fields needed by evaluateAlerts are set.
func newTestService(store ResourceStore, containerSvc *container.Service, cb EventCallback) *Service {
	return &Service{
		store:         store,
		containerSvc:  containerSvc,
		logger:        slog.Default(),
		eventCallback: cb,
	}
}

// buildContainerSvc builds a container.Service backed by the given mock store.
func buildContainerSvc(cs *mockContainerStore) *container.Service {
	return container.NewService(container.Deps{
		Store:  cs,
		Logger: slog.Default(),
	})
}

// baseConfig returns a minimal enabled alert config.
func baseConfig(containerID int64) *ResourceAlertConfig {
	return &ResourceAlertConfig{
		ContainerID:  containerID,
		Enabled:      true,
		CPUThreshold: 80.0,
		MemThreshold: 80.0,
		AlertState:   AlertStateNormal,
	}
}

// snap builds a snapshot with the given CPU/memory values.
func snap(containerID int64, cpu float64, memUsed, memLimit int64) *ResourceSnapshot {
	return &ResourceSnapshot{
		ContainerID: containerID,
		CPUPercent:  cpu,
		MemUsed:     memUsed,
		MemLimit:    memLimit,
		Timestamp:   time.Now(),
	}
}

// storedConfig reads the alert config back from the mock store.
func storedConfig(t *testing.T, store *mockResourceStore, containerID int64) *ResourceAlertConfig {
	t.Helper()
	cfg, err := store.GetAlertConfig(context.Background(), containerID)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

// ---------------------------------------------------------------------------
// evaluateAlerts — alert state machine tests
// ---------------------------------------------------------------------------

func TestService_evaluateAlerts_CPUBreachTransitionsToAlertAfterTwoConsecutive(t *testing.T) {
	const containerID = int64(1)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// First breach: consecutive count goes to 1 — not enough to alert yet.
	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 0, 0))
	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateNormal, cfg.AlertState, "single breach must not trigger alert")
	assert.Equal(t, 1, cfg.CPUConsecutiveBreaches)

	// Second breach: consecutive count reaches 2 — state must flip to AlertStateCPU.
	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 0, 0))
	cfg = storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateCPU, cfg.AlertState)
	assert.Equal(t, 2, cfg.CPUConsecutiveBreaches)
}

func TestService_evaluateAlerts_MemoryBreachTransitionsToAlertAfterTwoConsecutive(t *testing.T) {
	const containerID = int64(2)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// 90 % of limit → breaching MemThreshold=80.
	s := snap(containerID, 0, 90, 100)

	svc.evaluateAlerts(ctx, s)
	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateNormal, cfg.AlertState)
	assert.Equal(t, 1, cfg.MemConsecutiveBreaches)

	svc.evaluateAlerts(ctx, s)
	cfg = storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateMemory, cfg.AlertState)
	assert.Equal(t, 2, cfg.MemConsecutiveBreaches)
}

func TestService_evaluateAlerts_BothBreachingTransitionsToBothAlert(t *testing.T) {
	const containerID = int64(3)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// CPU=90 %, mem=90/100 → both above threshold.
	s := snap(containerID, 90.0, 90, 100)

	svc.evaluateAlerts(ctx, s)
	svc.evaluateAlerts(ctx, s)

	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateBoth, cfg.AlertState)
	assert.Equal(t, 2, cfg.CPUConsecutiveBreaches)
	assert.Equal(t, 2, cfg.MemConsecutiveBreaches)
}

func TestService_evaluateAlerts_BreachCountResetsOnRecovery(t *testing.T) {
	const containerID = int64(4)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// One breach to accumulate count.
	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 90, 100))

	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, 1, cfg.CPUConsecutiveBreaches)
	assert.Equal(t, 1, cfg.MemConsecutiveBreaches)

	// Values drop below threshold — both counters must reset to 0.
	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 10, 100))

	cfg = storedConfig(t, store, containerID)
	assert.Equal(t, 0, cfg.CPUConsecutiveBreaches, "CPU breach count must reset")
	assert.Equal(t, 0, cfg.MemConsecutiveBreaches, "memory breach count must reset")
	assert.Equal(t, AlertStateNormal, cfg.AlertState)
}

func TestService_evaluateAlerts_RecoveryEventFiredOnReturnToNormal(t *testing.T) {
	const containerID = int64(5)
	store := newMockResourceStore()

	// Start already in AlertStateCPU with 2 consecutive breaches.
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:            containerID,
		Enabled:                true,
		CPUThreshold:           80.0,
		MemThreshold:           80.0,
		AlertState:             AlertStateCPU,
		CPUConsecutiveBreaches: 2,
	}

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "web"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	// CPU drops below threshold — recovery should be emitted.
	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 0, 0))

	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateNormal, cfg.AlertState)

	require.Len(t, events, 1, "exactly one recovery event expected")
	assert.Equal(t, event.ResourceRecovery, events[0].eventType)
	assert.Equal(t, "cpu", events[0].data["recovered_type"])
	assert.Equal(t, containerID, events[0].data["container_id"])
}

func TestService_evaluateAlerts_AlertEventFiredOnCPUTransition(t *testing.T) {
	const containerID = int64(6)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "api"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 0, 0))
	// No alert yet — only one breach.
	assert.Empty(t, events)

	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 0, 0))
	// Second breach triggers alert.
	require.Len(t, events, 1)
	assert.Equal(t, event.ResourceAlert, events[0].eventType)
	assert.Equal(t, "cpu", events[0].data["alert_type"])
	assert.InDelta(t, 90.0, events[0].data["current_value"], 0.001)
	assert.InDelta(t, 80.0, events[0].data["threshold"], 0.001)
	assert.Equal(t, containerID, events[0].data["container_id"])
}

func TestService_evaluateAlerts_AlertEventFiredOnMemoryTransition(t *testing.T) {
	const containerID = int64(7)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "db"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	// mem = 90/100 = 90 % > 80 % threshold.
	s := snap(containerID, 10.0, 90, 100)
	svc.evaluateAlerts(ctx, s)
	svc.evaluateAlerts(ctx, s)

	require.Len(t, events, 1)
	assert.Equal(t, event.ResourceAlert, events[0].eventType)
	assert.Equal(t, "memory", events[0].data["alert_type"])
}

func TestService_evaluateAlerts_BothAlertsEmittedOnBothTransition(t *testing.T) {
	const containerID = int64(8)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = baseConfig(containerID)

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "worker"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	s := snap(containerID, 90.0, 90, 100)
	svc.evaluateAlerts(ctx, s)
	svc.evaluateAlerts(ctx, s)

	// Both CPU and memory alerts must be emitted separately.
	require.Len(t, events, 2)
	alertTypes := []string{events[0].data["alert_type"].(string), events[1].data["alert_type"].(string)}
	assert.ElementsMatch(t, []string{"cpu", "memory"}, alertTypes)
}

func TestService_evaluateAlerts_NoEventWhenStateUnchanged(t *testing.T) {
	const containerID = int64(9)
	store := newMockResourceStore()

	// Pre-set to AlertStateCPU so the state is already alerting.
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:            containerID,
		Enabled:                true,
		CPUThreshold:           80.0,
		MemThreshold:           80.0,
		AlertState:             AlertStateCPU,
		CPUConsecutiveBreaches: 3,
	}

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "cache"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	// Another CPU breach — state remains AlertStateCPU, no transition event.
	svc.evaluateAlerts(ctx, snap(containerID, 90.0, 0, 0))
	assert.Empty(t, events, "no event must be emitted when alert state is unchanged")
}

func TestService_evaluateAlerts_DisabledConfigSkipsEvaluation(t *testing.T) {
	const containerID = int64(10)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:  containerID,
		Enabled:      false,
		CPUThreshold: 80.0,
		MemThreshold: 80.0,
		AlertState:   AlertStateNormal,
	}

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, nil, cb)
	ctx := context.Background()

	// Repeated breaches should never produce alerts when disabled.
	svc.evaluateAlerts(ctx, snap(containerID, 99.0, 99, 100))
	svc.evaluateAlerts(ctx, snap(containerID, 99.0, 99, 100))
	svc.evaluateAlerts(ctx, snap(containerID, 99.0, 99, 100))

	// The store is never written to because evaluateAlerts returns early.
	// The in-memory state is still the original.
	cfg := store.alertConfigs[containerID]
	assert.Equal(t, AlertStateNormal, cfg.AlertState)
	assert.Empty(t, events)
}

func TestService_evaluateAlerts_NilConfigSkipsEvaluation(t *testing.T) {
	const containerID = int64(11)
	store := newMockResourceStore()
	// No config stored for this container.

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, nil, cb)
	svc.evaluateAlerts(context.Background(), snap(containerID, 99.0, 99, 100))

	assert.Empty(t, events, "nil config must produce no alerts")
}

// ---------------------------------------------------------------------------
// evaluateAlerts — memory percent calculation
// ---------------------------------------------------------------------------

func TestService_evaluateAlerts_MemLimitZeroNoDivisionByZero(t *testing.T) {
	const containerID = int64(12)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:  containerID,
		Enabled:      true,
		CPUThreshold: 80.0,
		MemThreshold: 50.0,
		AlertState:   AlertStateNormal,
	}

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// MemLimit=0 → memPercent must be treated as 0 (no division by zero).
	// Run twice to ensure it would trigger if memPercent were wrongly calculated.
	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 9999, 0))
	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 9999, 0))

	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateNormal, cfg.AlertState, "zero MemLimit must not trigger memory alert")
	assert.Equal(t, 0, cfg.MemConsecutiveBreaches)
}

func TestService_evaluateAlerts_MemPercentCalculatedCorrectly(t *testing.T) {
	const containerID = int64(13)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:  containerID,
		Enabled:      true,
		CPUThreshold: 99.0,  // high — won't trigger
		MemThreshold: 70.0,
		AlertState:   AlertStateNormal,
	}

	svc := newTestService(store, nil, nil)
	ctx := context.Background()

	// 75/100 = 75 % → above 70 % threshold.
	s := snap(containerID, 10.0, 75, 100)
	svc.evaluateAlerts(ctx, s)
	svc.evaluateAlerts(ctx, s)

	cfg := storedConfig(t, store, containerID)
	assert.Equal(t, AlertStateMemory, cfg.AlertState, "75%% mem usage must breach 70%% threshold")
}

func TestService_evaluateAlerts_RecoveryTypeIsBothWhenPrevStateWasBoth(t *testing.T) {
	const containerID = int64(14)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:            containerID,
		Enabled:                true,
		CPUThreshold:           80.0,
		MemThreshold:           80.0,
		AlertState:             AlertStateBoth,
		CPUConsecutiveBreaches: 3,
		MemConsecutiveBreaches: 3,
	}

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "proxy"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	// Both metrics drop below threshold → recovery from AlertStateBoth.
	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 10, 100))

	require.Len(t, events, 1)
	assert.Equal(t, event.ResourceRecovery, events[0].eventType)
	assert.Equal(t, "both", events[0].data["recovered_type"])
}

func TestService_evaluateAlerts_RecoveryTypeIsMemoryWhenPrevStateWasMemory(t *testing.T) {
	const containerID = int64(15)
	store := newMockResourceStore()
	store.alertConfigs[containerID] = &ResourceAlertConfig{
		ContainerID:            containerID,
		Enabled:                true,
		CPUThreshold:           80.0,
		MemThreshold:           80.0,
		AlertState:             AlertStateMemory,
		MemConsecutiveBreaches: 2,
	}

	containerStore := newMockContainerStore(&container.Container{ID: containerID, Name: "scheduler"})
	containerSvc := buildContainerSvc(containerStore)

	var events []capturedEvent
	cb := func(eventType string, data interface{}) {
		if m, ok := data.(map[string]interface{}); ok {
			events = append(events, capturedEvent{eventType: eventType, data: m})
		}
	}

	svc := newTestService(store, containerSvc, cb)
	ctx := context.Background()

	svc.evaluateAlerts(ctx, snap(containerID, 10.0, 10, 100))

	require.Len(t, events, 1)
	assert.Equal(t, event.ResourceRecovery, events[0].eventType)
	assert.Equal(t, "memory", events[0].data["recovered_type"])
}

// ---------------------------------------------------------------------------
// GetHistory — time range to granularity mapping
// ---------------------------------------------------------------------------

func TestService_GetHistory_TimeRangeToGranularityMapping(t *testing.T) {
	tests := []struct {
		timeRange           string
		expectedGranularity Granularity
	}{
		{"1h", GranularityRaw},
		{"6h", Granularity1m},
		{"24h", Granularity5m},
		{"7d", Granularity1h},
		{"unknown", GranularityRaw},
		{"", GranularityRaw},
	}

	for _, tc := range tests {
		t.Run(tc.timeRange, func(t *testing.T) {
			store := &granularityCapturingStore{}
			svc := newTestService(store, nil, nil)

			_, gran, err := svc.GetHistory(context.Background(), 1, tc.timeRange)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedGranularity, gran)
			assert.Equal(t, tc.expectedGranularity, store.capturedGranularity,
				"granularity passed to store must match returned granularity")
		})
	}
}

// granularityCapturingStore records the granularity argument from ListSnapshotsAggregated.
type granularityCapturingStore struct {
	mockResourceStore
	capturedGranularity Granularity
}

func (s *granularityCapturingStore) ListSnapshotsAggregated(_ context.Context, _ int64, _, _ time.Time, g Granularity) ([]*ResourceSnapshot, error) {
	s.capturedGranularity = g
	return nil, nil
}
