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

package heartbeat

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mockStore — in-memory HeartbeatStore for testing
// ---------------------------------------------------------------------------

type mockStore struct {
	mu         sync.Mutex
	nextID     int64
	heartbeats map[int64]*Heartbeat
	byUUID     map[string]*Heartbeat
	pings      []*HeartbeatPing
	executions []*HeartbeatExecution
	// overdue is injected per-test for checkDeadlines scenarios
	overdue []*Heartbeat
}

func newMockStore() *mockStore {
	return &mockStore{
		heartbeats: make(map[int64]*Heartbeat),
		byUUID:     make(map[string]*Heartbeat),
	}
}

// seed inserts a heartbeat directly, bypassing service logic.
func (m *mockStore) seed(h *Heartbeat) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	h.ID = m.nextID
	cp := *h
	m.heartbeats[cp.ID] = &cp
	m.byUUID[cp.UUID] = &cp
}

func (m *mockStore) CreateHeartbeat(_ context.Context, h *Heartbeat) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	h.ID = m.nextID
	cp := *h
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = time.Now()
	m.heartbeats[cp.ID] = &cp
	m.byUUID[cp.UUID] = &cp
	return cp.ID, nil
}

func (m *mockStore) GetHeartbeatByID(_ context.Context, id int64) (*Heartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return nil, nil
	}
	cp := *h
	return &cp, nil
}

func (m *mockStore) GetHeartbeatByUUID(_ context.Context, uuid string) (*Heartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.byUUID[uuid]
	if !ok {
		return nil, nil
	}
	cp := *h
	return &cp, nil
}

func (m *mockStore) ListHeartbeats(_ context.Context, _ ListHeartbeatsOpts) ([]*Heartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Heartbeat, 0, len(m.heartbeats))
	for _, h := range m.heartbeats {
		cp := *h
		out = append(out, &cp)
	}
	return out, nil
}

func (m *mockStore) UpdateHeartbeat(_ context.Context, id int64, input UpdateHeartbeatInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return errors.New("not found")
	}
	if input.Name != nil {
		h.Name = *input.Name
	}
	if input.IntervalSeconds != nil {
		h.IntervalSeconds = *input.IntervalSeconds
	}
	if input.GraceSeconds != nil {
		h.GraceSeconds = *input.GraceSeconds
	}
	h.UpdatedAt = time.Now()
	// keep byUUID in sync
	m.byUUID[h.UUID] = h
	return nil
}

func (m *mockStore) DeleteHeartbeat(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return errors.New("not found")
	}
	delete(m.byUUID, h.UUID)
	delete(m.heartbeats, id)
	return nil
}

func (m *mockStore) UpdateHeartbeatState(_ context.Context, id int64,
	status HeartbeatStatus, alertState AlertState,
	lastPingAt *time.Time, nextDeadlineAt *time.Time, currentRunStartedAt *time.Time,
	lastExitCode *int, lastDurationMs *int64,
	consecutiveFailures, consecutiveSuccesses int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return errors.New("not found")
	}
	h.Status = status
	h.AlertState = alertState
	h.LastPingAt = lastPingAt
	h.NextDeadlineAt = nextDeadlineAt
	h.CurrentRunStartedAt = currentRunStartedAt
	h.LastExitCode = lastExitCode
	h.LastDurationMs = lastDurationMs
	h.ConsecutiveFailures = consecutiveFailures
	h.ConsecutiveSuccesses = consecutiveSuccesses
	h.UpdatedAt = time.Now()
	m.byUUID[h.UUID] = h
	return nil
}

func (m *mockStore) PauseHeartbeat(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return errors.New("not found")
	}
	h.Status = StatusPaused
	m.byUUID[h.UUID] = h
	return nil
}

func (m *mockStore) ResumeHeartbeat(_ context.Context, id int64, nextDeadlineAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.heartbeats[id]
	if !ok {
		return errors.New("not found")
	}
	h.Status = StatusUp
	h.NextDeadlineAt = &nextDeadlineAt
	m.byUUID[h.UUID] = h
	return nil
}

func (m *mockStore) ListOverdueHeartbeats(_ context.Context, _ time.Time) ([]*Heartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.overdue, nil
}

func (m *mockStore) CountActiveHeartbeats(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, h := range m.heartbeats {
		if h.Active {
			count++
		}
	}
	return count, nil
}

func (m *mockStore) InsertPing(_ context.Context, p *HeartbeatPing) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	p.ID = m.nextID
	cp := *p
	m.pings = append(m.pings, &cp)
	return cp.ID, nil
}

func (m *mockStore) ListPings(_ context.Context, heartbeatID int64, opts ListPingsOpts) ([]*HeartbeatPing, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*HeartbeatPing
	for _, p := range m.pings {
		if p.HeartbeatID == heartbeatID {
			cp := *p
			out = append(out, &cp)
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

func (m *mockStore) InsertExecution(_ context.Context, e *HeartbeatExecution) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	e.ID = m.nextID
	cp := *e
	m.executions = append(m.executions, &cp)
	return cp.ID, nil
}

func (m *mockStore) UpdateExecution(_ context.Context, id int64, completedAt *time.Time, durationMs *int64, exitCode *int, outcome ExecutionOutcome, payload *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.executions {
		if e.ID == id {
			e.CompletedAt = completedAt
			e.DurationMs = durationMs
			e.ExitCode = exitCode
			e.Outcome = outcome
			e.Payload = payload
			return nil
		}
	}
	return errors.New("execution not found")
}

func (m *mockStore) GetCurrentExecution(_ context.Context, heartbeatID int64) (*HeartbeatExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// The "current" execution is the last in_progress one for this heartbeat.
	var current *HeartbeatExecution
	for _, e := range m.executions {
		if e.HeartbeatID == heartbeatID && e.Outcome == OutcomeInProgress {
			cp := *e
			current = &cp
		}
	}
	return current, nil
}

func (m *mockStore) ListExecutions(_ context.Context, heartbeatID int64, opts ListExecutionsOpts) ([]*HeartbeatExecution, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*HeartbeatExecution
	for _, e := range m.executions {
		if e.HeartbeatID == heartbeatID {
			cp := *e
			out = append(out, &cp)
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

func (m *mockStore) DeletePingsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (m *mockStore) DeleteExecutionsBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

// pingsFor returns all pings stored for the given heartbeat ID.
func (m *mockStore) pingsFor(heartbeatID int64) []*HeartbeatPing {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*HeartbeatPing
	for _, p := range m.pings {
		if p.HeartbeatID == heartbeatID {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out
}

// executionsFor returns all executions stored for the given heartbeat ID.
func (m *mockStore) executionsFor(heartbeatID int64) []*HeartbeatExecution {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*HeartbeatExecution
	for _, e := range m.executions {
		if e.HeartbeatID == heartbeatID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// mockLicense implements LicenseChecker.
type mockLicense struct {
	canCreate      bool
	canStorePayload bool
}

func (l *mockLicense) CanCreateHeartbeat(_ int) bool { return l.canCreate }
func (l *mockLicense) CanStorePayload() bool          { return l.canStorePayload }

func newService(store *mockStore, lc LicenseChecker) *Service {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewService(Deps{
		Store:          store,
		Logger:         logger,
		LicenseChecker: lc,
	})
}

// seedHeartbeat creates a heartbeat in the mock store ready for ping tests.
func seedHeartbeat(store *mockStore, uuid string, status HeartbeatStatus, alertState AlertState) *Heartbeat {
	h := &Heartbeat{
		UUID:            uuid,
		Name:            "test-heartbeat",
		Status:          status,
		AlertState:      alertState,
		IntervalSeconds: 300,
		GraceSeconds:    60,
		Active:          true,
	}
	store.seed(h)
	return h
}

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// CreateHeartbeat tests
// ---------------------------------------------------------------------------

func TestService_CreateHeartbeat_ValidInput(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	input := CreateHeartbeatInput{
		Name:            "my-job",
		IntervalSeconds: 300,
		GraceSeconds:    60,
	}
	h, err := svc.CreateHeartbeat(context.Background(), input, "uuid-001")
	require.NoError(t, err)
	require.NotNil(t, h)

	assert.Equal(t, "uuid-001", h.UUID)
	assert.Equal(t, "my-job", h.Name)
	assert.Equal(t, StatusNew, h.Status)
	assert.Equal(t, AlertNormal, h.AlertState)
	assert.Equal(t, 300, h.IntervalSeconds)
	assert.Equal(t, 60, h.GraceSeconds)
	assert.True(t, h.Active)
	assert.Greater(t, h.ID, int64(0))
}

func TestService_CreateHeartbeat_InvalidName(t *testing.T) {
	cases := []struct {
		name  string
		input CreateHeartbeatInput
	}{
		{
			name:  "empty name",
			input: CreateHeartbeatInput{Name: "", IntervalSeconds: 300, GraceSeconds: 0},
		},
		{
			name:  "name too long",
			input: CreateHeartbeatInput{Name: strings.Repeat("x", MaxNameLength+1), IntervalSeconds: 300, GraceSeconds: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			svc := newService(store, &mockLicense{canCreate: true})
			_, err := svc.CreateHeartbeat(context.Background(), tc.input, "uuid-x")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestService_CreateHeartbeat_InvalidInterval(t *testing.T) {
	cases := []struct {
		name  string
		input CreateHeartbeatInput
	}{
		{
			name:  "interval below minimum",
			input: CreateHeartbeatInput{Name: "job", IntervalSeconds: MinIntervalSeconds - 1, GraceSeconds: 0},
		},
		{
			name:  "interval above maximum",
			input: CreateHeartbeatInput{Name: "job", IntervalSeconds: MaxIntervalSeconds + 1, GraceSeconds: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			svc := newService(store, &mockLicense{canCreate: true})
			_, err := svc.CreateHeartbeat(context.Background(), tc.input, "uuid-x")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestService_CreateHeartbeat_GraceExceedsInterval(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	input := CreateHeartbeatInput{
		Name:            "job",
		IntervalSeconds: 300,
		GraceSeconds:    301, // grace > interval
	}
	_, err := svc.CreateHeartbeat(context.Background(), input, "uuid-x")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestService_CreateHeartbeat_LicenseLimitReached(t *testing.T) {
	store := newMockStore()
	// Pre-fill store with heartbeats up to the limit.
	lc := &DefaultLicenseChecker{MaxHeartbeats: 2}
	seedHeartbeat(store, "existing-1", StatusUp, AlertNormal)
	seedHeartbeat(store, "existing-2", StatusUp, AlertNormal)

	svc := newService(store, lc)
	input := CreateHeartbeatInput{Name: "new-job", IntervalSeconds: 300, GraceSeconds: 0}
	_, err := svc.CreateHeartbeat(context.Background(), input, "uuid-new")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLimitReached)
}

// ---------------------------------------------------------------------------
// ProcessPing tests
// ---------------------------------------------------------------------------

func TestService_ProcessPing_NewToUp(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-ping-1", StatusNew, AlertNormal)
	before := time.Now()

	result, err := svc.ProcessPing(context.Background(), "uuid-ping-1", "127.0.0.1", "GET", nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	after := time.Now()

	assert.Equal(t, StatusUp, result.Status)
	assert.Equal(t, 1, result.ConsecutiveSuccesses)
	assert.Equal(t, 0, result.ConsecutiveFailures)

	// Deadline = now + interval + grace
	require.NotNil(t, result.NextDeadlineAt)
	expectedMin := before.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	expectedMax := after.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	assert.True(t, !result.NextDeadlineAt.Before(expectedMin), "deadline too early")
	assert.True(t, !result.NextDeadlineAt.After(expectedMax), "deadline too late")

	// Ping recorded
	pings := store.pingsFor(result.ID)
	require.Len(t, pings, 1)
	assert.Equal(t, PingSuccess, pings[0].PingType)
}

func TestService_ProcessPing_ConsecutiveSuccessesIncrement(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	seedHeartbeat(store, "uuid-ping-2", StatusUp, AlertNormal)

	for i := 1; i <= 3; i++ {
		result, err := svc.ProcessPing(context.Background(), "uuid-ping-2", "127.0.0.1", "GET", nil)
		require.NoError(t, err)
		assert.Equal(t, i, result.ConsecutiveSuccesses, "ping #%d", i)
		assert.Equal(t, 0, result.ConsecutiveFailures)
	}
}

func TestService_ProcessPing_DownToUpRecovery(t *testing.T) {
	store := newMockStore()

	var alertCalls []string
	svc := newService(store, &mockLicense{canCreate: true})
	svc.SetAlertCallback(func(h *Heartbeat, alertType string, _ map[string]interface{}) {
		alertCalls = append(alertCalls, alertType)
	})

	seedHeartbeat(store, "uuid-ping-3", StatusDown, AlertAlerting)

	result, err := svc.ProcessPing(context.Background(), "uuid-ping-3", "127.0.0.1", "POST", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusUp, result.Status)
	assert.Equal(t, AlertNormal, result.AlertState)
	assert.Equal(t, 0, result.ConsecutiveFailures)

	require.Len(t, alertCalls, 1, "recovery alert callback must fire exactly once")
	assert.Equal(t, "recovery", alertCalls[0])
}

func TestService_ProcessPing_UpToUpNoRecoveryAlert(t *testing.T) {
	store := newMockStore()

	var alertCalls int
	svc := newService(store, &mockLicense{canCreate: true})
	svc.SetAlertCallback(func(_ *Heartbeat, _ string, _ map[string]interface{}) {
		alertCalls++
	})

	seedHeartbeat(store, "uuid-ping-4", StatusUp, AlertNormal)

	_, err := svc.ProcessPing(context.Background(), "uuid-ping-4", "127.0.0.1", "GET", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, alertCalls, "no alert callback for up→up transition")
}

func TestService_ProcessPing_StartedToUpCompletionCalculatesDuration(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	startedAt := time.Now().Add(-5 * time.Second)
	h := &Heartbeat{
		UUID:                "uuid-ping-5",
		Name:                "job",
		Status:              StatusStarted,
		AlertState:          AlertNormal,
		IntervalSeconds:     300,
		GraceSeconds:        60,
		CurrentRunStartedAt: &startedAt,
		Active:              true,
	}
	store.seed(h)

	// Insert an in-progress execution to be completed.
	execID, err := store.InsertExecution(context.Background(), &HeartbeatExecution{
		HeartbeatID: h.ID,
		StartedAt:   &startedAt,
		Outcome:     OutcomeInProgress,
	})
	require.NoError(t, err)
	_ = execID

	result, err := svc.ProcessPing(context.Background(), "uuid-ping-5", "127.0.0.1", "GET", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusUp, result.Status)
	require.NotNil(t, result.LastDurationMs, "duration_ms must be set after start→up")
	assert.GreaterOrEqual(t, *result.LastDurationMs, int64(5000), "duration must be at least 5s in ms")
	assert.Nil(t, result.CurrentRunStartedAt, "CurrentRunStartedAt must be cleared")

	// The in-progress execution must have been updated to success.
	execs := store.executionsFor(h.ID)
	require.Len(t, execs, 1)
	assert.Equal(t, OutcomeSuccess, execs[0].Outcome)
	assert.NotNil(t, execs[0].CompletedAt)
}

func TestService_ProcessPing_UnknownUUID(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	_, err := svc.ProcessPing(context.Background(), "nonexistent-uuid", "127.0.0.1", "GET", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHeartbeatNotFound)
}

// ---------------------------------------------------------------------------
// ProcessStartPing tests
// ---------------------------------------------------------------------------

func TestService_ProcessStartPing_SetsStartedStatus(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-start-1", StatusUp, AlertNormal)
	before := time.Now()

	result, err := svc.ProcessStartPing(context.Background(), "uuid-start-1", "127.0.0.1", "GET")
	require.NoError(t, err)

	after := time.Now()

	assert.Equal(t, StatusStarted, result.Status)
	require.NotNil(t, result.CurrentRunStartedAt)
	assert.False(t, result.CurrentRunStartedAt.Before(before))
	assert.False(t, result.CurrentRunStartedAt.After(after))

	// Deadline must be interval+grace from now.
	require.NotNil(t, result.NextDeadlineAt)
	expectedMin := before.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	expectedMax := after.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	assert.True(t, !result.NextDeadlineAt.Before(expectedMin))
	assert.True(t, !result.NextDeadlineAt.After(expectedMax))

	// Ping recorded as "start".
	pings := store.pingsFor(result.ID)
	require.Len(t, pings, 1)
	assert.Equal(t, PingStart, pings[0].PingType)

	// A new in-progress execution must have been created.
	execs := store.executionsFor(result.ID)
	require.Len(t, execs, 1)
	assert.Equal(t, OutcomeInProgress, execs[0].Outcome)
}

func TestService_ProcessStartPing_TimesOutPreviousExecution(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-start-2", StatusStarted, AlertNormal)

	// Insert a stale in-progress execution.
	_, err := store.InsertExecution(context.Background(), &HeartbeatExecution{
		HeartbeatID: h.ID,
		Outcome:     OutcomeInProgress,
	})
	require.NoError(t, err)

	_, err = svc.ProcessStartPing(context.Background(), "uuid-start-2", "127.0.0.1", "GET")
	require.NoError(t, err)

	execs := store.executionsFor(h.ID)
	// Two executions: the timed-out one and the new in-progress.
	require.Len(t, execs, 2)

	// First one should be timed out.
	assert.Equal(t, OutcomeTimeout, execs[0].Outcome)
	// Second one should be new in-progress.
	assert.Equal(t, OutcomeInProgress, execs[1].Outcome)
}

// ---------------------------------------------------------------------------
// ProcessExitCodePing tests
// ---------------------------------------------------------------------------

func TestService_ProcessExitCodePing_ExitCodeZeroSuccess(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	seedHeartbeat(store, "uuid-exit-1", StatusUp, AlertNormal)

	result, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-1", 0, "127.0.0.1", "GET", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusUp, result.Status)
	assert.Equal(t, AlertNormal, result.AlertState)
	assert.Equal(t, 1, result.ConsecutiveSuccesses)
	assert.Equal(t, 0, result.ConsecutiveFailures)
	require.NotNil(t, result.LastExitCode)
	assert.Equal(t, 0, *result.LastExitCode)
}

func TestService_ProcessExitCodePing_NonZeroExitCodeFiresAlert(t *testing.T) {
	store := newMockStore()

	var alertCalls []string
	svc := newService(store, &mockLicense{canCreate: true})
	svc.SetAlertCallback(func(_ *Heartbeat, alertType string, _ map[string]interface{}) {
		alertCalls = append(alertCalls, alertType)
	})

	seedHeartbeat(store, "uuid-exit-2", StatusUp, AlertNormal)

	result, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-2", 1, "127.0.0.1", "GET", nil)
	require.NoError(t, err)

	assert.Equal(t, StatusUp, result.Status)
	assert.Equal(t, AlertAlerting, result.AlertState)
	assert.Equal(t, 1, result.ConsecutiveFailures)
	assert.Equal(t, 0, result.ConsecutiveSuccesses)

	require.Len(t, alertCalls, 1)
	assert.Equal(t, "alert", alertCalls[0])
}

func TestService_ProcessExitCodePing_ConsecutiveFailuresIncrement(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	seedHeartbeat(store, "uuid-exit-3", StatusUp, AlertNormal)

	for i := 1; i <= 3; i++ {
		result, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-3", 2, "127.0.0.1", "GET", nil)
		require.NoError(t, err)
		// Mock does not persist state between calls; seed the updated state.
		// Verify within each call that counter increments by checking store state.
		stored := store.heartbeats[result.ID]
		assert.Equal(t, i, stored.ConsecutiveFailures, "ping #%d", i)
		assert.Equal(t, 0, stored.ConsecutiveSuccesses)
	}
}

func TestService_ProcessExitCodePing_InvalidExitCodeBelow0(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	seedHeartbeat(store, "uuid-exit-4", StatusUp, AlertNormal)

	_, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-4", -1, "127.0.0.1", "GET", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidExitCode)
}

func TestService_ProcessExitCodePing_InvalidExitCodeAbove255(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	seedHeartbeat(store, "uuid-exit-5", StatusUp, AlertNormal)

	_, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-5", 256, "127.0.0.1", "GET", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidExitCode)
}

func TestService_ProcessExitCodePing_StartedToUpCalculatesDuration(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	startedAt := time.Now().Add(-10 * time.Second)
	h := &Heartbeat{
		UUID:                "uuid-exit-6",
		Name:                "job",
		Status:              StatusStarted,
		AlertState:          AlertNormal,
		IntervalSeconds:     300,
		GraceSeconds:        60,
		CurrentRunStartedAt: &startedAt,
		Active:              true,
	}
	store.seed(h)

	// In-progress execution to be completed.
	_, err := store.InsertExecution(context.Background(), &HeartbeatExecution{
		HeartbeatID: h.ID,
		StartedAt:   &startedAt,
		Outcome:     OutcomeInProgress,
	})
	require.NoError(t, err)

	result, err := svc.ProcessExitCodePing(context.Background(), "uuid-exit-6", 0, "127.0.0.1", "GET", nil)
	require.NoError(t, err)

	require.NotNil(t, result.LastDurationMs)
	assert.GreaterOrEqual(t, *result.LastDurationMs, int64(10000))
	assert.Nil(t, result.CurrentRunStartedAt)

	execs := store.executionsFor(h.ID)
	require.Len(t, execs, 1)
	assert.Equal(t, OutcomeSuccess, execs[0].Outcome)
}

// ---------------------------------------------------------------------------
// checkDeadlines tests
// ---------------------------------------------------------------------------

func TestService_checkDeadlines_OverdueTransitionsToDown(t *testing.T) {
	store := newMockStore()

	var alertCalls []string
	svc := newService(store, &mockLicense{canCreate: true})
	svc.SetAlertCallback(func(h *Heartbeat, alertType string, _ map[string]interface{}) {
		alertCalls = append(alertCalls, alertType)
	})

	h := seedHeartbeat(store, "uuid-deadline-1", StatusUp, AlertNormal)

	// Inject the heartbeat as overdue.
	store.overdue = []*Heartbeat{store.heartbeats[h.ID]}

	svc.checkDeadlines(context.Background())

	// Inspect stored state.
	updated := store.heartbeats[h.ID]
	assert.Equal(t, StatusDown, updated.Status)
	assert.Equal(t, AlertAlerting, updated.AlertState)
	assert.Equal(t, 1, updated.ConsecutiveFailures)
	assert.Equal(t, 0, updated.ConsecutiveSuccesses)

	require.Len(t, alertCalls, 1)
	assert.Equal(t, "alert", alertCalls[0])
}

func TestService_checkDeadlines_StartedStatusTimesOutExecution(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-deadline-2", StatusStarted, AlertNormal)

	// In-progress execution that will be timed out by the deadline checker.
	_, err := store.InsertExecution(context.Background(), &HeartbeatExecution{
		HeartbeatID: h.ID,
		Outcome:     OutcomeInProgress,
	})
	require.NoError(t, err)

	store.overdue = []*Heartbeat{store.heartbeats[h.ID]}

	svc.checkDeadlines(context.Background())

	updated := store.heartbeats[h.ID]
	assert.Equal(t, StatusDown, updated.Status)
	assert.Equal(t, AlertAlerting, updated.AlertState)

	execs := store.executionsFor(h.ID)
	require.Len(t, execs, 1)
	assert.Equal(t, OutcomeTimeout, execs[0].Outcome)
}

func TestService_checkDeadlines_NoOverdueNoAlert(t *testing.T) {
	store := newMockStore()

	var alertCalls int
	svc := newService(store, &mockLicense{canCreate: true})
	svc.SetAlertCallback(func(_ *Heartbeat, _ string, _ map[string]interface{}) {
		alertCalls++
	})

	store.overdue = nil // no overdue heartbeats

	svc.checkDeadlines(context.Background())

	assert.Equal(t, 0, alertCalls)
}

// ---------------------------------------------------------------------------
// PauseHeartbeat / ResumeHeartbeat tests
// ---------------------------------------------------------------------------

func TestService_PauseHeartbeat_Success(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-pause-1", StatusUp, AlertNormal)

	result, err := svc.PauseHeartbeat(context.Background(), h.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusPaused, result.Status)
}

func TestService_ResumeHeartbeat_RejectsNonPaused(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-resume-1", StatusUp, AlertNormal)

	_, err := svc.ResumeHeartbeat(context.Background(), h.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestService_ResumeHeartbeat_SetsNewDeadline(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	h := seedHeartbeat(store, "uuid-resume-2", StatusPaused, AlertNormal)
	before := time.Now()

	result, err := svc.ResumeHeartbeat(context.Background(), h.ID)
	require.NoError(t, err)

	after := time.Now()

	assert.Equal(t, StatusUp, result.Status)
	require.NotNil(t, result.NextDeadlineAt)

	expectedMin := before.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	expectedMax := after.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	assert.True(t, !result.NextDeadlineAt.Before(expectedMin), "deadline too early")
	assert.True(t, !result.NextDeadlineAt.After(expectedMax), "deadline too late")
}

func TestService_ResumeHeartbeat_NotFound(t *testing.T) {
	store := newMockStore()
	svc := newService(store, &mockLicense{canCreate: true})

	_, err := svc.ResumeHeartbeat(context.Background(), 9999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHeartbeatNotFound)
}

// ---------------------------------------------------------------------------
// filterPayload tests
// ---------------------------------------------------------------------------

func TestService_filterPayload_CommunityDropsPayload(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: false}
	svc := newService(store, lc)

	p := "some data"
	result := svc.filterPayload(&p)
	assert.Nil(t, result, "community license must drop payload")
}

func TestService_filterPayload_ProPreservesPayload(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: true}
	svc := newService(store, lc)

	p := "important log output"
	result := svc.filterPayload(&p)
	require.NotNil(t, result)
	assert.Equal(t, p, *result)
}

func TestService_filterPayload_TruncatesAtMaxPayloadBytes(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: true}
	svc := newService(store, lc)

	oversized := strings.Repeat("a", MaxPayloadBytes+500)
	result := svc.filterPayload(&oversized)
	require.NotNil(t, result)
	assert.Len(t, *result, MaxPayloadBytes)
}

func TestService_filterPayload_NilPayloadReturnsNil(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: true}
	svc := newService(store, lc)

	result := svc.filterPayload(nil)
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// Payload stored/dropped in ping record
// ---------------------------------------------------------------------------

func TestService_ProcessPing_PayloadDroppedForCommunity(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: false}
	svc := newService(store, lc)

	seedHeartbeat(store, "uuid-payload-1", StatusUp, AlertNormal)

	payload := "job output"
	_, err := svc.ProcessPing(context.Background(), "uuid-payload-1", "127.0.0.1", "POST", &payload)
	require.NoError(t, err)

	// Find the ping for this heartbeat.
	allPings := store.pings
	require.NotEmpty(t, allPings)
	assert.Nil(t, allPings[len(allPings)-1].Payload, "community license must not store payload")
}

func TestService_ProcessExitCodePing_PayloadPreservedForPro(t *testing.T) {
	store := newMockStore()
	lc := &mockLicense{canCreate: true, canStorePayload: true}
	svc := newService(store, lc)

	seedHeartbeat(store, "uuid-payload-2", StatusUp, AlertNormal)

	payload := "exit log"
	_, err := svc.ProcessExitCodePing(context.Background(), "uuid-payload-2", 0, "127.0.0.1", "POST", &payload)
	require.NoError(t, err)

	allPings := store.pings
	require.NotEmpty(t, allPings)
	lastPing := allPings[len(allPings)-1]
	require.NotNil(t, lastPing.Payload)
	assert.Equal(t, payload, *lastPing.Payload)
}
