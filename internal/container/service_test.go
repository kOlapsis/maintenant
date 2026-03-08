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

package container

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// svcStore — full-featured in-memory ContainerStore for service tests.
// Named "svcStore" to avoid collision with the minimal mockStore in uptime_test.go.
// ---------------------------------------------------------------------------

type svcStore struct {
	mu          sync.Mutex
	containers  map[string]*Container // keyed by ExternalID
	byID        map[int64]*Container
	transitions []*StateTransition
	nextID      int64
	nextTxnID   int64

	// injection points for error simulation
	errGetByExternalID  error
	errUpdate           error
	errInsertTransition error
	errArchive          error
}

func newSvcStore() *svcStore {
	return &svcStore{
		containers: make(map[string]*Container),
		byID:       make(map[int64]*Container),
		nextID:     1,
		nextTxnID:  1,
	}
}

func (m *svcStore) seed(c *Container) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.ID == 0 {
		c.ID = m.nextID
		m.nextID++
	}
	clone := *c
	m.containers[c.ExternalID] = &clone
	m.byID[clone.ID] = &clone
}

func (m *svcStore) InsertContainer(_ context.Context, c *Container) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	clone := *c
	clone.ID = id
	m.containers[c.ExternalID] = &clone
	m.byID[id] = &clone
	return id, nil
}

func (m *svcStore) UpdateContainer(_ context.Context, c *Container) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errUpdate != nil {
		return m.errUpdate
	}
	clone := *c
	m.containers[c.ExternalID] = &clone
	m.byID[c.ID] = &clone
	return nil
}

func (m *svcStore) GetContainerByExternalID(_ context.Context, externalID string) (*Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errGetByExternalID != nil {
		return nil, m.errGetByExternalID
	}
	c, ok := m.containers[externalID]
	if !ok {
		return nil, nil
	}
	clone := *c
	return &clone, nil
}

func (m *svcStore) GetContainerByID(_ context.Context, id int64) (*Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	clone := *c
	return &clone, nil
}

func (m *svcStore) ListContainers(_ context.Context, opts ListContainersOpts) ([]*Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*Container
	for _, c := range m.containers {
		if !opts.IncludeArchived && c.Archived {
			continue
		}
		clone := *c
		result = append(result, &clone)
	}
	return result, nil
}

func (m *svcStore) ArchiveContainer(_ context.Context, externalID string, archivedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errArchive != nil {
		return m.errArchive
	}
	c, ok := m.containers[externalID]
	if !ok {
		return nil
	}
	c.Archived = true
	c.ArchivedAt = &archivedAt
	m.byID[c.ID].Archived = true
	m.byID[c.ID].ArchivedAt = &archivedAt
	return nil
}

func (m *svcStore) DeleteContainerByID(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byID[id]
	if !ok {
		return nil
	}
	delete(m.containers, c.ExternalID)
	delete(m.byID, id)
	return nil
}

func (m *svcStore) InsertTransition(_ context.Context, t *StateTransition) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errInsertTransition != nil {
		return 0, m.errInsertTransition
	}
	clone := *t
	clone.ID = m.nextTxnID
	m.nextTxnID++
	m.transitions = append(m.transitions, &clone)
	return clone.ID, nil
}

func (m *svcStore) ListTransitionsByContainer(_ context.Context, containerID int64, _ ListTransitionsOpts) ([]*StateTransition, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*StateTransition
	for _, t := range m.transitions {
		if t.ContainerID == containerID {
			clone := *t
			result = append(result, &clone)
		}
	}
	return result, len(result), nil
}

func (m *svcStore) CountRestartsSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return 0, nil
}

func (m *svcStore) GetTransitionsInWindow(_ context.Context, _ int64, _, _ time.Time) ([]*StateTransition, error) {
	return nil, nil
}

func (m *svcStore) DeleteTransitionsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *svcStore) DeleteArchivedContainersBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// storedState retrieves the current state from the store for a container
// identified by ExternalID. Returns "" if not found.
func (m *svcStore) storedState(externalID string) ContainerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[externalID]
	if !ok {
		return ""
	}
	return c.State
}

// transitionsFor returns all recorded transitions for a given container ID.
func (m *svcStore) transitionsFor(containerID int64) []*StateTransition {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*StateTransition
	for _, t := range m.transitions {
		if t.ContainerID == containerID {
			clone := *t
			result = append(result, &clone)
		}
	}
	return result
}

// isArchived reports whether the container with the given ExternalID is marked archived.
func (m *svcStore) isArchived(externalID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[externalID]
	return ok && c.Archived
}

// storedHealthStatus retrieves the current health status pointer from the store.
func (m *svcStore) storedHealthStatus(externalID string) *HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[externalID]
	if !ok {
		return nil
	}
	return c.HealthStatus
}

// ---------------------------------------------------------------------------
// Mock LogFetcher
// ---------------------------------------------------------------------------

type mockLogFetcher struct {
	snippet string
	err     error
	calls   int
}

func (f *mockLogFetcher) FetchLogSnippet(_ context.Context, _ string) (string, error) {
	f.calls++
	return f.snippet, f.err
}

// ---------------------------------------------------------------------------
// Mock RestartChecker
// ---------------------------------------------------------------------------

type mockRestartChecker struct {
	result interface{}
	err    error
	calls  int
}

func (r *mockRestartChecker) Check(_ context.Context, _ *Container) (interface{}, error) {
	r.calls++
	return r.result, r.err
}

// ---------------------------------------------------------------------------
// Mock RuntimeDiscoverer
// ---------------------------------------------------------------------------

type mockDiscoverer struct {
	containers []*Container
	err        error
	calls      int
}

func (d *mockDiscoverer) DiscoverAll(_ context.Context) ([]*Container, error) {
	d.calls++
	return d.containers, d.err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestService(store *svcStore, opts ...func(*Deps)) *Service {
	d := Deps{
		Store:  store,
		Logger: slog.Default(),
	}
	for _, o := range opts {
		o(&d)
	}
	return NewService(d)
}

func makeTestContainer(externalID string, state ContainerState) *Container {
	return &Container{
		ExternalID:        externalID,
		Name:              externalID,
		Image:             "test-image:latest",
		State:             state,
		FirstSeenAt:       time.Now(),
		LastStateChangeAt: time.Now(),
	}
}

func makeTestEvent(action, externalID string) ContainerEvent {
	return ContainerEvent{
		Action:     action,
		ExternalID: externalID,
		Timestamp:  time.Now(),
	}
}

// extID builds a deterministic 64-char external ID string for use in tests.
// Docker container IDs are 64 hex characters. We pad a short label to that length.
func extID(label string) string {
	const length = 64
	for len(label) < length {
		label += label
	}
	return label[:length]
}

// ---------------------------------------------------------------------------
// ProcessEvent — state machine tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_StartTransitionsToRunning(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("a"), StateExited)
	c.ID = 10
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Equal(t, StateRunning, store.storedState(c.ExternalID))

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	assert.Equal(t, StateExited, transitions[0].PreviousState)
	assert.Equal(t, StateRunning, transitions[0].NewState)
}

func TestService_ProcessEvent_DieWithZeroExitCodeSetsCompleted(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("b"), StateRunning)
	c.ID = 11
	store.seed(c)

	svc := newTestService(store)

	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "0"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, StateCompleted, store.storedState(c.ExternalID))
}

func TestService_ProcessEvent_DieWithNonZeroExitCodeSetsExited(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("c"), StateRunning)
	c.ID = 12
	store.seed(c)

	svc := newTestService(store)

	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "1"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
}

func TestService_ProcessEvent_StopAfterCompletedIsNoop(t *testing.T) {
	// Docker sends "die" before "stop". When die exits with code 0 the state
	// becomes Completed; the subsequent stop must not overwrite it with Exited.
	store := newSvcStore()
	c := makeTestContainer(extID("d"), StateCompleted)
	c.ID = 13
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("stop", c.ExternalID))

	assert.Equal(t, StateCompleted, store.storedState(c.ExternalID))
	assert.Empty(t, store.transitionsFor(c.ID))
}

func TestService_ProcessEvent_StopWhenNotCompletedSetsExited(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("e"), StateRunning)
	c.ID = 14
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("stop", c.ExternalID))

	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	assert.Equal(t, StateExited, transitions[0].NewState)
}

func TestService_ProcessEvent_SameStateSuppressesDuplicate(t *testing.T) {
	// A "start" event on an already-running container must be a no-op:
	// no state update and no transition recorded.
	store := newSvcStore()
	c := makeTestContainer(extID("f"), StateRunning)
	c.ID = 15
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Equal(t, StateRunning, store.storedState(c.ExternalID))
	assert.Empty(t, store.transitionsFor(c.ID))
}

func TestService_ProcessEvent_UnknownContainerStartTriggersReconcile(t *testing.T) {
	store := newSvcStore()
	discoverer := &mockDiscoverer{}
	svc := newTestService(store, func(d *Deps) {
		d.Discoverer = discoverer
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", extID("newcontainer")))

	assert.Equal(t, 1, discoverer.calls, "Reconcile should be triggered once for an unknown start event")
}

func TestService_ProcessEvent_UnknownContainerStopIsIgnored(t *testing.T) {
	// stop/die for an unknown container must be silently dropped;
	// Reconcile must NOT be triggered.
	store := newSvcStore()
	discoverer := &mockDiscoverer{}
	svc := newTestService(store, func(d *Deps) {
		d.Discoverer = discoverer
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("stop", extID("unknown")))

	assert.Equal(t, 0, discoverer.calls, "Reconcile must not be triggered for non-start events on unknown containers")
	assert.Empty(t, store.transitions)
}

// ---------------------------------------------------------------------------
// handleHealthChange tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_HealthChangeUpdatesContainer(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("g"), StateRunning)
	c.ID = 20
	store.seed(c)

	svc := newTestService(store)

	evt := makeTestEvent("health_status", c.ExternalID)
	evt.HealthStatus = string(HealthUnhealthy)
	svc.ProcessEvent(context.Background(), evt)

	h := store.storedHealthStatus(c.ExternalID)
	require.NotNil(t, h)
	assert.Equal(t, HealthUnhealthy, *h)
}

func TestService_ProcessEvent_HealthChangeRecordsTransition(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("h"), StateRunning)
	c.ID = 21
	store.seed(c)

	svc := newTestService(store)

	evt := makeTestEvent("health_status", c.ExternalID)
	evt.HealthStatus = string(HealthHealthy)
	svc.ProcessEvent(context.Background(), evt)

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	// State stays the same; only the health field transitions.
	assert.Equal(t, StateRunning, transitions[0].PreviousState)
	assert.Equal(t, StateRunning, transitions[0].NewState)
	require.NotNil(t, transitions[0].NewHealth)
	assert.Equal(t, HealthHealthy, *transitions[0].NewHealth)
}

// ---------------------------------------------------------------------------
// Restart detection tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_RestartFromExitedTriggersCheck(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("i"), StateExited)
	c.ID = 30
	store.seed(c)

	checker := &mockRestartChecker{result: nil} // nil = below threshold
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Equal(t, 1, checker.calls, "RestartChecker.Check must be called once on exited→running transition")
}

func TestService_ProcessEvent_RestartCheckBelowThresholdEmitsRecovery(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("j"), StateExited)
	c.ID = 31
	store.seed(c)

	checker := &mockRestartChecker{result: nil}

	var emittedEvents []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emittedEvents = append(emittedEvents, eventType)
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Contains(t, emittedEvents, "container.restart_recovery",
		"a recovery event must be emitted when restart count is below threshold")
}

func TestService_ProcessEvent_RestartCheckAboveThresholdEmitsAlert(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("k"), StateRestarting)
	c.ID = 32
	store.seed(c)

	alertPayload := map[string]interface{}{"container_id": int64(32), "restarts": 10}
	checker := &mockRestartChecker{result: alertPayload}

	var emittedEvents []string
	svc := newTestService(store, func(d *Deps) {
		d.RestartChecker = checker
		d.EventCallback = func(eventType string, _ interface{}) {
			emittedEvents = append(emittedEvents, eventType)
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Contains(t, emittedEvents, "container.restart_alert",
		"a restart alert event must be emitted when checker returns a non-nil result")
}

// ---------------------------------------------------------------------------
// handleDestroy tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_DestroyArchivesContainer(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("l"), StateExited)
	c.ID = 40
	store.seed(c)

	var archivedEvents []interface{}
	svc := newTestService(store, func(d *Deps) {
		d.EventCallback = func(eventType string, data interface{}) {
			if eventType == "container.archived" {
				archivedEvents = append(archivedEvents, data)
			}
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("destroy", c.ExternalID))

	assert.True(t, store.isArchived(c.ExternalID), "container must be marked archived after destroy event")
	assert.Len(t, archivedEvents, 1, "an archived event must be emitted")
}

func TestService_ProcessEvent_DestroyUnknownContainerIsNoop(t *testing.T) {
	store := newSvcStore()

	var archivedEvents []interface{}
	svc := newTestService(store, func(d *Deps) {
		d.EventCallback = func(eventType string, data interface{}) {
			if eventType == "container.archived" {
				archivedEvents = append(archivedEvents, data)
			}
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("destroy", extID("nope")))

	assert.Empty(t, archivedEvents, "no archived event should be emitted for an unknown container")
}

// ---------------------------------------------------------------------------
// Log snippet capture tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_DieCapturesLogSnippet(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("m"), StateRunning)
	c.ID = 50
	store.seed(c)

	fetcher := &mockLogFetcher{snippet: "panic: runtime error\ngoroutine 1 ...", err: nil}
	svc := newTestService(store, func(d *Deps) {
		d.LogFetcher = fetcher
	})

	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "1"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, 1, fetcher.calls, "FetchLogSnippet must be called on die events")

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	assert.Equal(t, fetcher.snippet, transitions[0].LogSnippet)
}

func TestService_ProcessEvent_DieFetchLogSnippetErrorIsNonFatal(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("n"), StateRunning)
	c.ID = 51
	store.seed(c)

	fetcher := &mockLogFetcher{err: errors.New("docker daemon unavailable")}
	svc := newTestService(store, func(d *Deps) {
		d.LogFetcher = fetcher
	})

	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "1"

	// Must not panic; the state change must still be persisted.
	require.NotPanics(t, func() {
		svc.ProcessEvent(context.Background(), evt)
	})
	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
}

func TestService_ProcessEvent_DieWithoutLogFetcherLeavesEmptySnippet(t *testing.T) {
	// When no LogFetcher is configured, die events must still produce a
	// transition with an empty LogSnippet.
	store := newSvcStore()
	c := makeTestContainer(extID("o"), StateRunning)
	c.ID = 52
	store.seed(c)

	svc := newTestService(store) // no LogFetcher

	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "2"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	assert.Empty(t, transitions[0].LogSnippet)
}

// ---------------------------------------------------------------------------
// isGracefulExitCode tests
// ---------------------------------------------------------------------------

func TestIsGracefulExitCode(t *testing.T) {
	graceful := []int{0, 137, 143}
	for _, code := range graceful {
		assert.True(t, isGracefulExitCode(code), "exit code %d should be graceful", code)
	}

	nonGraceful := []int{1, 2, 127, 255, -1, 130, 138}
	for _, code := range nonGraceful {
		assert.False(t, isGracefulExitCode(code), "exit code %d should not be graceful", code)
	}
}

// ---------------------------------------------------------------------------
// ListContainersGrouped tests
// ---------------------------------------------------------------------------

func TestService_ListContainersGrouped_GroupsByOrchestration(t *testing.T) {
	store := newSvcStore()

	// Two containers belonging to the same compose project.
	c1 := makeTestContainer(extID("aa"), StateRunning)
	c1.ID = 60
	c1.OrchestrationGroup = "myapp"
	c1.RuntimeType = "docker"

	c2 := makeTestContainer(extID("bb"), StateRunning)
	c2.ID = 61
	c2.OrchestrationGroup = "myapp"
	c2.RuntimeType = "docker"

	// One container without any group.
	c3 := makeTestContainer(extID("cc"), StateRunning)
	c3.ID = 62

	store.seed(c1)
	store.seed(c2)
	store.seed(c3)

	svc := newTestService(store)

	groups, total, archived, err := svc.ListContainersGrouped(context.Background(), ListContainersOpts{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 0, archived)

	// Find the "myapp" group.
	var myappGroup *ContainerGroup
	for _, g := range groups {
		if g.Name == "myapp" {
			myappGroup = g
			break
		}
	}
	require.NotNil(t, myappGroup, "expected a group named 'myapp'")
	assert.Len(t, myappGroup.Containers, 2)
	assert.Equal(t, "compose", myappGroup.Source)

	// The ungrouped container must form its own "Ungrouped" group.
	var ungrouped *ContainerGroup
	for _, g := range groups {
		if g.Name == "Ungrouped" {
			ungrouped = g
			break
		}
	}
	require.NotNil(t, ungrouped)
	assert.Len(t, ungrouped.Containers, 1)
}

func TestService_ListContainersGrouped_CountsArchived(t *testing.T) {
	store := newSvcStore()

	c1 := makeTestContainer(extID("dd"), StateRunning)
	c1.ID = 70
	c2 := makeTestContainer(extID("ee"), StateExited)
	c2.ID = 71
	c2.Archived = true
	now := time.Now()
	c2.ArchivedAt = &now

	store.seed(c1)
	store.seed(c2)

	svc := newTestService(store)

	_, total, archived, err := svc.ListContainersGrouped(context.Background(), ListContainersOpts{IncludeArchived: true})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, archived)
}

func TestService_ListContainersGrouped_CustomGroupTakesPrecedence(t *testing.T) {
	store := newSvcStore()

	c := makeTestContainer(extID("ff"), StateRunning)
	c.ID = 80
	c.OrchestrationGroup = "compose-project"
	c.CustomGroup = "my-custom-label"

	store.seed(c)

	svc := newTestService(store)

	groups, _, _, err := svc.ListContainersGrouped(context.Background(), ListContainersOpts{})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "my-custom-label", groups[0].Name)
	assert.Equal(t, "label", groups[0].Source)
}

// ---------------------------------------------------------------------------
// Event emission tests
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_StateChangedEventIsEmitted(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("gg"), StateExited)
	c.ID = 90
	store.seed(c)

	var emitted []string
	svc := newTestService(store, func(d *Deps) {
		d.EventCallback = func(eventType string, _ interface{}) {
			emitted = append(emitted, eventType)
		}
	})

	svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))

	assert.Contains(t, emitted, "container.state_changed")
}

func TestService_ProcessEvent_NoEventCallbackIsNilSafe(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("hh"), StateExited)
	c.ID = 91
	store.seed(c)

	// No EventCallback configured — must not panic.
	svc := newTestService(store)
	require.NotPanics(t, func() {
		svc.ProcessEvent(context.Background(), makeTestEvent("start", c.ExternalID))
	})
}

// ---------------------------------------------------------------------------
// parseExitCode tests
// ---------------------------------------------------------------------------

func TestParseExitCode_ValidAndInvalid(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"0", 0},
		{"1", 1},
		{"137", 137},
		{"255", 255},
		{"", 0},    // empty → fmt.Sscanf returns 0
		{"abc", 0}, // non-numeric → fmt.Sscanf returns 0
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseExitCode(tc.input))
		})
	}
}

// ---------------------------------------------------------------------------
// Die exit code → graceful/non-graceful state mapping
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_DieWithSIGKILL137SetsCompleted(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("ii"), StateRunning)
	c.ID = 100
	store.seed(c)

	svc := newTestService(store)
	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "137"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, StateCompleted, store.storedState(c.ExternalID),
		"exit code 137 (SIGKILL from docker stop) is considered graceful")
}

func TestService_ProcessEvent_DieWithSIGTERM143SetsCompleted(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("jj"), StateRunning)
	c.ID = 101
	store.seed(c)

	svc := newTestService(store)
	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "143"
	svc.ProcessEvent(context.Background(), evt)

	assert.Equal(t, StateCompleted, store.storedState(c.ExternalID),
		"exit code 143 (SIGTERM) is considered graceful")
}

// ---------------------------------------------------------------------------
// Kill action maps to Exited
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_KillSetsExited(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("kk"), StateRunning)
	c.ID = 110
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("kill", c.ExternalID))

	assert.Equal(t, StateExited, store.storedState(c.ExternalID))
}

// ---------------------------------------------------------------------------
// Pause / Unpause
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_PauseSetsStatePaused(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("ll"), StateRunning)
	c.ID = 120
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("pause", c.ExternalID))

	assert.Equal(t, StatePaused, store.storedState(c.ExternalID))
}

func TestService_ProcessEvent_UnpauseSetsStateRunning(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("mm"), StatePaused)
	c.ID = 121
	store.seed(c)

	svc := newTestService(store)
	svc.ProcessEvent(context.Background(), makeTestEvent("unpause", c.ExternalID))

	assert.Equal(t, StateRunning, store.storedState(c.ExternalID))
}

// ---------------------------------------------------------------------------
// ExitCode recorded in transition
// ---------------------------------------------------------------------------

func TestService_ProcessEvent_ExitCodeStoredInTransition(t *testing.T) {
	store := newSvcStore()
	c := makeTestContainer(extID("nn"), StateRunning)
	c.ID = 130
	store.seed(c)

	svc := newTestService(store)
	evt := makeTestEvent("die", c.ExternalID)
	evt.ExitCode = "42"
	svc.ProcessEvent(context.Background(), evt)

	transitions := store.transitionsFor(c.ID)
	require.Len(t, transitions, 1)
	require.NotNil(t, transitions[0].ExitCode)
	assert.Equal(t, 42, *transitions[0].ExitCode)
}
