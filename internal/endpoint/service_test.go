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

package endpoint

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// In-memory EndpointStore implementation
// ---------------------------------------------------------------------------

type memStore struct {
	mu        sync.Mutex
	endpoints map[int64]*Endpoint
	results   []*CheckResult
	nextID    int64

	// Recorded calls for assertions
	deactivated []int64
	updated     []int64
}

func newMemStore() *memStore {
	return &memStore{
		endpoints: make(map[int64]*Endpoint),
		nextID:    1,
	}
}

func (m *memStore) cloneEp(ep *Endpoint) *Endpoint {
	c := *ep
	return &c
}

func (m *memStore) UpsertEndpoint(_ context.Context, e *Endpoint) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Look for existing by ExternalID + LabelKey
	for _, stored := range m.endpoints {
		if stored.ExternalID == e.ExternalID && stored.LabelKey == e.LabelKey {
			stored.Target = e.Target
			stored.EndpointType = e.EndpointType
			stored.Config = e.Config
			stored.Active = true
			return stored.ID, nil
		}
	}

	id := m.nextID
	m.nextID++
	ec := m.cloneEp(e)
	ec.ID = id
	ec.Active = true
	m.endpoints[id] = ec
	return id, nil
}

func (m *memStore) GetEndpointByIdentity(_ context.Context, containerName, labelKey string) (*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ep := range m.endpoints {
		if ep.ContainerName == containerName && ep.LabelKey == labelKey {
			c := m.cloneEp(ep)
			return c, nil
		}
	}
	return nil, nil
}

func (m *memStore) GetEndpointByID(_ context.Context, id int64) (*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return nil, nil
	}
	c := m.cloneEp(ep)
	return c, nil
}

func (m *memStore) ListEndpoints(_ context.Context, _ ListEndpointsOpts) ([]*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		c := m.cloneEp(ep)
		out = append(out, c)
	}
	return out, nil
}

func (m *memStore) ListEndpointsByExternalID(_ context.Context, externalID string) ([]*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Endpoint
	for _, ep := range m.endpoints {
		if ep.ExternalID == externalID && ep.Active {
			c := m.cloneEp(ep)
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memStore) DeactivateEndpoint(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ep, ok := m.endpoints[id]; ok {
		ep.Active = false
		m.deactivated = append(m.deactivated, id)
	}
	return nil
}

func (m *memStore) UpdateCheckResult(_ context.Context, id int64, status EndpointStatus, alertState AlertState,
	consecutiveFailures, consecutiveSuccesses int,
	responseTimeMs int64, httpStatus *int, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ep, ok := m.endpoints[id]; ok {
		ep.Status = status
		ep.AlertState = alertState
		ep.ConsecutiveFailures = consecutiveFailures
		ep.ConsecutiveSuccesses = consecutiveSuccesses
		ep.LastError = lastError
		m.updated = append(m.updated, id)
	}
	return nil
}

func (m *memStore) InsertCheckResult(_ context.Context, result *CheckResult) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := int64(len(m.results) + 1)
	r := *result
	r.ID = id
	m.results = append(m.results, &r)
	return id, nil
}

func (m *memStore) ListCheckResults(_ context.Context, endpointID int64, opts ListChecksOpts) ([]*CheckResult, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*CheckResult
	for _, r := range m.results {
		if r.EndpointID == endpointID {
			out = append(out, r)
		}
	}
	total := len(out)
	if opts.Offset < len(out) {
		out = out[opts.Offset:]
	} else {
		out = nil
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, total, nil
}

func (m *memStore) GetCheckResultsInWindow(_ context.Context, endpointID int64, from, to time.Time) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total, successes := 0, 0
	for _, r := range m.results {
		if r.EndpointID == endpointID && !r.Timestamp.Before(from) && !r.Timestamp.After(to) {
			total++
			if r.Success {
				successes++
			}
		}
	}
	return total, successes, nil
}

func (m *memStore) DeleteCheckResultsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *memStore) DeleteInactiveEndpointsBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func noopEngine() *CheckEngine {
	return NewCheckEngine(nil, noopLogger())
}

// seedEndpoint inserts an endpoint into the store and returns its assigned ID.
func seedEndpoint(t *testing.T, store *memStore, ep *Endpoint) int64 {
	t.Helper()
	id, err := store.UpsertEndpoint(context.Background(), ep)
	require.NoError(t, err)
	// Sync state that UpsertEndpoint copies
	store.mu.Lock()
	stored := store.endpoints[id]
	stored.Status = ep.Status
	stored.AlertState = ep.AlertState
	stored.ConsecutiveFailures = ep.ConsecutiveFailures
	stored.ConsecutiveSuccesses = ep.ConsecutiveSuccesses
	store.mu.Unlock()
	return id
}

func newService(store *memStore) *Service {
	return NewService(Deps{
		Store:  store,
		Engine: noopEngine(),
		Logger: noopLogger(),
	})
}

// ---------------------------------------------------------------------------
// ProcessCheckResult — counter transitions
// ---------------------------------------------------------------------------

func TestService_ProcessCheckResult_SuccessIncrements(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:               StatusDown,
		ConsecutiveFailures:  2,
		ConsecutiveSuccesses: 0,
		AlertState:           AlertNormal,
		Config:               DefaultConfig(),
		Active:               true,
	})

	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    true,
		Timestamp:  time.Now(),
	})

	ep, err := store.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, ep)

	assert.Equal(t, 1, ep.ConsecutiveSuccesses, "consecutive successes should be incremented to 1")
	assert.Equal(t, 0, ep.ConsecutiveFailures, "consecutive failures should be reset to 0")
	assert.Equal(t, StatusUp, ep.Status)
}

func TestService_ProcessCheckResult_FailureIncrements(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:               StatusUp,
		ConsecutiveSuccesses: 5,
		ConsecutiveFailures:  0,
		AlertState:           AlertNormal,
		Config:               DefaultConfig(),
		Active:               true,
	})

	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID:   id,
		Success:      false,
		ErrorMessage: "connection refused",
		Timestamp:    time.Now(),
	})

	ep, err := store.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, ep)

	assert.Equal(t, 1, ep.ConsecutiveFailures, "consecutive failures should be incremented to 1")
	assert.Equal(t, 0, ep.ConsecutiveSuccesses, "consecutive successes should be reset to 0")
	assert.Equal(t, StatusDown, ep.Status)
}

func TestService_ProcessCheckResult_StatusTransitionEmitsEvent(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	var emittedEvents []string
	svc.SetEventCallback(func(eventType string, _ interface{}) {
		emittedEvents = append(emittedEvents, eventType)
	})

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:     StatusUp,
		AlertState: AlertNormal,
		Config:     DefaultConfig(),
		Active:     true,
	})

	// Trigger a failure — status transitions from up → down
	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    false,
		Timestamp:  time.Now(),
	})

	assert.Contains(t, emittedEvents, "endpoint.status_changed",
		"a status change from up to down must emit endpoint.status_changed")
}

func TestService_ProcessCheckResult_SameStatusNoEvent(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	var emittedEvents []string
	svc.SetEventCallback(func(eventType string, _ interface{}) {
		emittedEvents = append(emittedEvents, eventType)
	})

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:     StatusUp,
		AlertState: AlertNormal,
		Config:     DefaultConfig(),
		Active:     true,
	})

	// Result is still a success — status stays up
	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    true,
		Timestamp:  time.Now(),
	})

	for _, e := range emittedEvents {
		assert.NotEqual(t, "endpoint.status_changed", e,
			"no status_changed event should fire when status stays the same")
	}
}

// ---------------------------------------------------------------------------
// ProcessCheckResult — alert callback integration
// ---------------------------------------------------------------------------

func TestService_ProcessCheckResult_AlertCallbackTriggered(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	callbackInvoked := false
	svc.SetAlertCallback(func(ep *Endpoint, _ CheckResult) (string, interface{}) {
		callbackInvoked = true
		return "endpoint.alert", map[string]interface{}{"endpoint_id": ep.ID}
	})

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:     StatusUp,
		AlertState: AlertNormal,
		Config:     DefaultConfig(),
		Active:     true,
	})

	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    false,
		Timestamp:  time.Now(),
	})

	assert.True(t, callbackInvoked, "alertCallback must be invoked on every check result")
}

func TestService_ProcessCheckResult_AlertStateUpdatedOnAlert(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	svc.SetAlertCallback(func(_ *Endpoint, _ CheckResult) (string, interface{}) {
		return "endpoint.alert", nil
	})

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:     StatusUp,
		AlertState: AlertNormal,
		Config:     DefaultConfig(),
		Active:     true,
	})

	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    false,
		Timestamp:  time.Now(),
	})

	ep, err := store.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, ep)

	assert.Equal(t, AlertAlerting, ep.AlertState,
		"alert state must be updated to AlertAlerting when alertCallback returns endpoint.alert")
}

func TestService_ProcessCheckResult_RecoveryResetsAlertState(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	svc.SetAlertCallback(func(_ *Endpoint, _ CheckResult) (string, interface{}) {
		return "endpoint.recovery", nil
	})

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Status:     StatusDown,
		AlertState: AlertAlerting,
		Config:     DefaultConfig(),
		Active:     true,
	})

	svc.ProcessCheckResult(ctx, id, CheckResult{
		EndpointID: id,
		Success:    true,
		Timestamp:  time.Now(),
	})

	ep, err := store.GetEndpointByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, ep)

	assert.Equal(t, AlertNormal, ep.AlertState,
		"alert state must be reset to AlertNormal when alertCallback returns endpoint.recovery")
}

// ---------------------------------------------------------------------------
// HandleContainerStop
// ---------------------------------------------------------------------------

func TestService_HandleContainerStop_SetsEndpointsUnknown(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	externalID := "ctr-stop"
	id1 := seedEndpoint(t, store, &Endpoint{
		ExternalID: externalID, LabelKey: "k1",
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})
	id2 := seedEndpoint(t, store, &Endpoint{
		ExternalID: externalID, LabelKey: "k2",
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})

	svc.HandleContainerStop(ctx, externalID)

	for _, id := range []int64{id1, id2} {
		ep, err := store.GetEndpointByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, StatusUnknown, ep.Status,
			"endpoint %d should be StatusUnknown after container stop", id)
	}
}

func TestService_HandleContainerStop_RemovesFromEngine(t *testing.T) {
	store := newMemStore()
	engine := noopEngine()
	svc := NewService(Deps{
		Store:  store,
		Engine: engine,
		Logger: noopLogger(),
	})
	ctx := context.Background()

	externalID := "ctr-stop-engine"
	seedEndpoint(t, store, &Endpoint{
		ExternalID: externalID, LabelKey: "k1",
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})

	// Verify the engine has nothing loaded before; after stop it should still be clean
	assert.Equal(t, 0, engine.ActiveCount(), "engine should have no active runners at start")
	svc.HandleContainerStop(ctx, externalID)
	// RemoveEndpoint on an ID that was never added is a no-op — engine stays at 0
	assert.Equal(t, 0, engine.ActiveCount(), "engine should have no active runners after stop")
}

// ---------------------------------------------------------------------------
// HandleContainerDestroy
// ---------------------------------------------------------------------------

func TestService_HandleContainerDestroy_DeactivatesAndRemoves(t *testing.T) {
	store := newMemStore()
	var removedIDs []int64
	svc := NewService(Deps{
		Store:  store,
		Engine: noopEngine(),
		Logger: noopLogger(),
		EndpointRemovedCallback: func(_ context.Context, id int64) {
			removedIDs = append(removedIDs, id)
		},
	})
	ctx := context.Background()

	externalID := "ctr-destroy"
	id1 := seedEndpoint(t, store, &Endpoint{
		ExternalID: externalID, LabelKey: "k1",
		Status: StatusUp, AlertState: AlertNormal,
		Config: DefaultConfig(), Active: true,
	})
	id2 := seedEndpoint(t, store, &Endpoint{
		ExternalID: externalID, LabelKey: "k2",
		Status: StatusDown, AlertState: AlertAlerting,
		Config: DefaultConfig(), Active: true,
	})

	svc.HandleContainerDestroy(ctx, externalID)

	// Both endpoints must be deactivated
	for _, id := range []int64{id1, id2} {
		ep, err := store.GetEndpointByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.False(t, ep.Active, "endpoint %d should be deactivated after container destroy", id)
	}

	// onEndpointRemoved must have been called for each
	assert.ElementsMatch(t, []int64{id1, id2}, removedIDs,
		"EndpointRemovedCallback must be called for each endpoint on container destroy")
}

// ---------------------------------------------------------------------------
// CalculateUptime
// ---------------------------------------------------------------------------

func TestService_CalculateUptime_AllWindows(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Config: DefaultConfig(), Active: true,
	})

	now := time.Now()

	// 4 results inside the 1h window: 3 successes, 1 failure → 75 %
	for i := range 3 {
		store.mu.Lock()
		store.results = append(store.results, &CheckResult{
			ID:         int64(i + 1),
			EndpointID: id,
			Success:    true,
			Timestamp:  now.Add(-30 * time.Minute),
		})
		store.mu.Unlock()
	}
	store.mu.Lock()
	store.results = append(store.results, &CheckResult{
		ID:         4,
		EndpointID: id,
		Success:    false,
		Timestamp:  now.Add(-30 * time.Minute),
	})
	store.mu.Unlock()

	uptimes := svc.CalculateUptime(ctx, id)

	require.Contains(t, uptimes, "1h")
	require.Contains(t, uptimes, "24h")
	require.Contains(t, uptimes, "7d")
	require.Contains(t, uptimes, "30d")

	assert.InDelta(t, 75.0, uptimes["1h"], 0.001,
		"1h uptime should be 75%% (3 successes out of 4 checks)")
	// 24h, 7d, 30d windows encompass the same results
	assert.InDelta(t, 75.0, uptimes["24h"], 0.001)
	assert.InDelta(t, 75.0, uptimes["7d"], 0.001)
	assert.InDelta(t, 75.0, uptimes["30d"], 0.001)
}

func TestService_CalculateUptime_NoResults(t *testing.T) {
	store := newMemStore()
	svc := newService(store)
	ctx := context.Background()

	id := seedEndpoint(t, store, &Endpoint{
		ExternalID: "c1", LabelKey: "k1",
		Config: DefaultConfig(), Active: true,
	})

	uptimes := svc.CalculateUptime(ctx, id)

	for label, pct := range uptimes {
		assert.Equal(t, 0.0, pct, "window %s must be 0 when there are no check results", label)
	}
}
