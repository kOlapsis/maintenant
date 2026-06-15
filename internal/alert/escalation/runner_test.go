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

package escalation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- in-memory mocks ---

// runStore implements escalation.Store with in-memory state, sufficient to
// exercise the runtime state machine.
type runStore struct {
	mu             sync.Mutex
	policies       map[string]*Policy
	runs           map[string]*Run
	deliveries     map[string]*Delivery
	insertDelivErr error // injectable to simulate UNIQUE violation
}

func newRunStore() *runStore {
	return &runStore{
		policies:   map[string]*Policy{},
		runs:       map[string]*Run{},
		deliveries: map[string]*Delivery{},
	}
}

func (s *runStore) addPolicy(p *Policy) *Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID = uid.New()
	if !p.Active {
		// default to active when caller leaves the field unset
		p.Active = true
	}
	s.policies[p.ID] = p
	return p
}

func (s *runStore) listRuns() []*Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Run, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, r)
	}
	return out
}

func (s *runStore) listDeliveries() []*Delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Delivery, 0, len(s.deliveries))
	for _, d := range s.deliveries {
		out = append(out, d)
	}
	return out
}

// onlyRunID returns the id of the sole run in the store (tests create exactly one).
func (s *runStore) onlyRunID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.runs {
		return id
	}
	return ""
}

// Policy methods (subset needed by Runner; the rest panic to surface
// unexpected calls during tests).

func (s *runStore) InsertPolicy(_ context.Context, _ *Policy) (string, error) {
	panic("not used in runner tests")
}
func (s *runStore) UpdatePolicy(_ context.Context, _ *Policy) error { return nil }
func (s *runStore) SelectPolicy(_ context.Context, id string) (*Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policies[id], nil
}
func (s *runStore) SelectPolicies(_ context.Context, activeOnly bool) ([]*Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Policy
	for _, p := range s.policies {
		if !activeOnly || p.Active {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *runStore) DeletePolicy(_ context.Context, _ string) error     { return nil }
func (s *runStore) CountActivePolicies(_ context.Context) (int, error) { return 0, nil }
func (s *runStore) SelectRunsByPolicy(_ context.Context, _ string, _ int, _ string) ([]*Run, error) {
	return nil, nil
}
func (s *runStore) SelectRunDeliveries(_ context.Context, _ string) ([]*Delivery, error) {
	return nil, nil
}
func (s *runStore) BulkDeactivateAllPolicies(_ context.Context) error        { return nil }
func (s *runStore) BulkRestorePoliciesFromDowngrade(_ context.Context) error { return nil }
func (s *runStore) BulkStopActiveRuns(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (s *runStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, _ time.Time) error {
	return nil
}

// Run lifecycle.

func (s *runStore) InsertRun(_ context.Context, r *Run) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = uid.New()
	cp := *r
	s.runs[r.ID] = &cp
	return r.ID, nil
}

func (s *runStore) SelectRun(_ context.Context, id string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.runs[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (s *runStore) SelectRunsByAlert(_ context.Context, alertID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.AlertID == alertID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *runStore) SelectActiveRunsByAlert(_ context.Context, alertID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.AlertID != alertID {
			continue
		}
		if r.Status == RunStatusActive || r.Status == RunStatusPausedByMaintenance {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *runStore) SelectDueRuns(_ context.Context, now time.Time) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.Status != RunStatusActive && r.Status != RunStatusPausedByMaintenance {
			continue
		}
		if r.NextActionAt == nil || r.NextActionAt.After(now) {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	return out, nil
}

func (s *runStore) UpdateRunProgress(_ context.Context, runID string, lastExecutedLevelIndex int, nextActionAt *time.Time, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	r.LastExecutedLevelIndex = lastExecutedLevelIndex
	r.NextActionAt = nextActionAt
	r.Status = status
	return nil
}

func (s *runStore) TerminateRun(_ context.Context, runID string, status string, endedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	r.Status = status
	r.EndedAt = &endedAt
	r.NextActionAt = nil
	return nil
}

func (s *runStore) PauseRunForMaintenance(_ context.Context, runID string, recheckAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	r.Status = RunStatusPausedByMaintenance
	r.NextActionAt = &recheckAt
	return nil
}

func (s *runStore) ResumeRunFromMaintenance(_ context.Context, runID string, nextActionAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	r.Status = RunStatusActive
	r.NextActionAt = &nextActionAt
	return nil
}

// Delivery lifecycle.

func (s *runStore) InsertDelivery(_ context.Context, d *Delivery) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.insertDelivErr != nil {
		err := s.insertDelivErr
		s.insertDelivErr = nil
		return "", err
	}
	// Enforce UNIQUE (run_id, level_index, channel_id) like the SQL constraint.
	for _, existing := range s.deliveries {
		if existing.RunID == d.RunID && existing.LevelIndex == d.LevelIndex {
			if (existing.ChannelID == nil && d.ChannelID == nil) ||
				(existing.ChannelID != nil && d.ChannelID != nil && *existing.ChannelID == *d.ChannelID) {
				return "", ErrDeliveryDuplicate
			}
		}
	}
	d.ID = uid.New()
	cp := *d
	s.deliveries[d.ID] = &cp
	return d.ID, nil
}

func (s *runStore) UpdateDelivery(_ context.Context, d *Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.deliveries[d.ID]; ok {
		existing.Status = d.Status
		existing.Error = d.Error
		existing.SentAt = d.SentAt
	}
	return nil
}

func (s *runStore) SelectOrphanPendingDeliveries(_ context.Context, before time.Time) ([]*Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Delivery
	for _, d := range s.deliveries {
		if d.Status == DeliveryStatusPending && d.AttemptStartedAt.Before(before) {
			cp := *d
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- alert store mock ---

type alertStoreMock struct {
	mu     sync.Mutex
	alerts map[string]*alert.Alert
}

func newAlertStoreMock() *alertStoreMock {
	return &alertStoreMock{alerts: map[string]*alert.Alert{}}
}
func (m *alertStoreMock) put(a *alert.Alert) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts[a.ID] = a
}
func (m *alertStoreMock) GetAlert(_ context.Context, id string) (*alert.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.alerts[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}
func (m *alertStoreMock) InsertAlert(_ context.Context, _ *alert.Alert) (string, error) {
	return "", nil
}
func (m *alertStoreMock) ListAlerts(_ context.Context, _ alert.ListAlertsOpts) ([]*alert.Alert, error) {
	return nil, nil
}
func (m *alertStoreMock) UpdateAlertStatus(_ context.Context, _ string, _ string, _ *time.Time, _ *string) error {
	return nil
}
func (m *alertStoreMock) UpdateAlertOnEscalation(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (m *alertStoreMock) GetActiveAlert(_ context.Context, _, _, _ string, _ string) (*alert.Alert, error) {
	return nil, nil
}
func (m *alertStoreMock) ListActiveAlerts(_ context.Context) ([]*alert.Alert, error) {
	return nil, nil
}
func (m *alertStoreMock) DeleteAlertsOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *alertStoreMock) AcknowledgeAlert(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (m *alertStoreMock) SetEscalatedAt(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *alertStoreMock) ListUnacknowledgedActiveAlerts(_ context.Context) ([]*alert.Alert, error) {
	return nil, nil
}

// --- channel store mock ---

type channelStoreMock struct {
	channels map[string]*alert.NotificationChannel
}

func newChannelStoreMock() *channelStoreMock {
	return &channelStoreMock{channels: map[string]*alert.NotificationChannel{}}
}
func (m *channelStoreMock) put(c *alert.NotificationChannel) {
	m.channels[c.ID] = c
}
func (m *channelStoreMock) GetChannel(_ context.Context, id string) (*alert.NotificationChannel, error) {
	if c, ok := m.channels[id]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}
func (m *channelStoreMock) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (string, error) {
	return "", nil
}
func (m *channelStoreMock) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *channelStoreMock) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (m *channelStoreMock) DeleteChannel(_ context.Context, _ string) error { return nil }
func (m *channelStoreMock) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (m *channelStoreMock) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "", nil
}
func (m *channelStoreMock) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *channelStoreMock) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// --- sender mock ---

type senderCall struct {
	AlertID   string
	ChannelID string
	Message   string
}

type fakeSender struct {
	mu       sync.Mutex
	calls    []senderCall
	failFor  map[string]bool // channel IDs that should fail
	sendErr  error
	delivers atomic.Int32
}

func newFakeSender() *fakeSender {
	return &fakeSender{failFor: map[string]bool{}}
}

func (f *fakeSender) SendNow(_ context.Context, a *alert.Alert, ch *alert.NotificationChannel) error {
	f.delivers.Add(1)
	f.mu.Lock()
	f.calls = append(f.calls, senderCall{AlertID: a.ID, ChannelID: ch.ID, Message: a.Message})
	failing := f.failFor[ch.ID]
	staticErr := f.sendErr
	f.mu.Unlock()
	if staticErr != nil {
		return staticErr
	}
	if failing {
		return errors.New("simulated send failure")
	}
	return nil
}

func (f *fakeSender) snapshot() []senderCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]senderCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// --- suppressor mock ---

type fakeSuppressor struct {
	mu          sync.Mutex
	suppressing bool
}

func (f *fakeSuppressor) set(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.suppressing = v
}

func (f *fakeSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.suppressing, nil
}

// --- harness ---

type harness struct {
	store    *runStore
	alerts   *alertStoreMock
	channels *channelStoreMock
	sender   *fakeSender
	supp     *fakeSuppressor
	service  *Service
	runner   *Runner
	now      time.Time
	clockMu  sync.Mutex
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := newRunStore()
	alerts := newAlertStoreMock()
	channels := newChannelStoreMock()
	sender := newFakeSender()
	supp := &fakeSuppressor{}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewService(store, channels, nil, supp, logger)

	h := &harness{
		store:    store,
		alerts:   alerts,
		channels: channels,
		sender:   sender,
		supp:     supp,
		service:  svc,
		now:      time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}
	h.runner = NewRunner(RunnerDeps{
		Store:         store,
		AlertStore:    alerts,
		ChannelStore:  channels,
		Notifier:      sender,
		Suppressor:    supp,
		Service:       svc,
		Logger:        logger,
		Clock:         h.clock,
		OrphanTimeout: time.Minute,
	})
	return h
}

func (h *harness) clock() time.Time {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	return h.now
}

func (h *harness) advance(d time.Duration) {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	h.now = h.now.Add(d)
}

// waitForDeliveries blocks until the sender has been invoked n times or the
// timeout expires (the runner dispatches via goroutines).
func (h *harness) waitForDeliveries(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if int(h.sender.delivers.Load()) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d deliveries, got %d", n, h.sender.delivers.Load())
}

// --- shared fixtures ---

func criticalAlert(id string) *alert.Alert {
	return &alert.Alert{
		ID:         id,
		Source:     alert.SourceContainer,
		AlertType:  "container_stopped",
		Severity:   alert.SeverityCritical,
		Status:     alert.StatusActive,
		Message:    "container x stopped",
		EntityType: "container",
		EntityID:   "42",
		EntityName: "x",
		FiredAt:    time.Now().UTC(),
	}
}

func policyTwoLevels() *Policy {
	return &Policy{
		Name:   "p1",
		Active: true,
		Filters: Filters{
			Severities: []string{alert.SeverityCritical},
		},
		Levels: []Level{
			{Order: 0, DelaySeconds: 60, ChannelIDs: []string{"1"}},
			{Order: 1, DelaySeconds: 180, ChannelIDs: []string{"1"}},
		},
	}
}

func defaultChannel() *alert.NotificationChannel {
	return &alert.NotificationChannel{ID: "1", Name: "slack-test", Type: "slack", URL: "https://example.test/x", Enabled: true}
}

// --- tests ---

func TestRunner_OnAlertCreated_NoMatch(t *testing.T) {
	h := newHarness(t)
	a := criticalAlert("1")
	a.Severity = alert.SeverityWarning // policy filter is critical-only
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())

	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))
	assert.Empty(t, h.store.listRuns())
}

func TestRunner_OnAlertCreated_StartsRun(t *testing.T) {
	h := newHarness(t)
	a := criticalAlert("1")
	h.alerts.put(a)
	p := h.store.addPolicy(policyTwoLevels())

	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	r := runs[0]
	assert.Equal(t, RunStatusActive, r.Status)
	assert.Equal(t, -1, r.LastExecutedLevelIndex)
	require.NotNil(t, r.PolicyID)
	assert.Equal(t, p.ID, *r.PolicyID)
	require.NotNil(t, r.NextActionAt)
	assert.Equal(t, h.now.Add(60*time.Second), *r.NextActionAt)
}

func TestRunner_OnAlertCreated_Idempotent(t *testing.T) {
	h := newHarness(t)
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())

	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))
	assert.Len(t, h.store.listRuns(), 1)
}

func TestRunner_EvaluateCycle_FiresLevelAndAdvances(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	h.advance(61 * time.Second) // first level due
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 1)

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	r := runs[0]
	assert.Equal(t, 0, r.LastExecutedLevelIndex)
	assert.Equal(t, RunStatusActive, r.Status)
	require.NotNil(t, r.NextActionAt)
	// next_action_at = current now + level[1].DelaySeconds (180s)
	assert.Equal(t, h.now.Add(180*time.Second), *r.NextActionAt)

	calls := h.sender.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "1", calls[0].ChannelID)
	assert.Equal(t, "1", calls[0].AlertID)
}

func TestRunner_EvaluateCycle_ExhaustedAfterLastLevel(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Tick 1: fire level 0
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 1)

	// Tick 2: fire level 1
	h.advance(181 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 2)

	// Tick 3: exhausted notif + run terminated
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 3) // exhausted notif

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, RunStatusExhausted, runs[0].Status)
	require.NotNil(t, runs[0].EndedAt)

	// Last delivery uses the special exhausted level index.
	deliveries := h.store.listDeliveries()
	var sawExhausted bool
	for _, d := range deliveries {
		if d.LevelIndex == specialLevelExhausted {
			sawExhausted = true
		}
	}
	assert.True(t, sawExhausted)
}

func TestRunner_OnAlertAcknowledged_StopsRunsAndDispatchesAck(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Fire level 0 first so ack has channels to notify.
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 1)

	require.NoError(t, h.runner.OnAlertAcknowledged(context.Background(), "1", alert.Acknowledgment{By: "alice", At: h.now}))
	h.waitForDeliveries(t, 2) // ack notification

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, RunStatusStoppedByAck, runs[0].Status)
	require.NotNil(t, runs[0].EndedAt)

	deliveries := h.store.listDeliveries()
	var ackCount int
	for _, d := range deliveries {
		if d.LevelIndex == specialLevelAck {
			ackCount++
		}
	}
	assert.Equal(t, 1, ackCount)
}

func TestRunner_OnAlertResolved_StopsRunsNoNotif(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	require.NoError(t, h.runner.OnAlertResolved(context.Background(), "1", h.now))

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, RunStatusStoppedByResolution, runs[0].Status)
	// No deliveries at all (run never reached a level, no resolve notif).
	assert.Empty(t, h.store.listDeliveries())
	assert.Empty(t, h.sender.snapshot())
}

func TestRunner_MaintenancePauseAndResume(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Suppressor is on when level 0 becomes due.
	h.supp.set(true)
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))

	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, RunStatusPausedByMaintenance, runs[0].Status)
	assert.Empty(t, h.sender.snapshot())

	// Maintenance window cleared. First post-pause cycle resumes; the run
	// becomes active and surfaces again next tick to actually fire.
	h.supp.set(false)
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	runs = h.store.listRuns()
	assert.Equal(t, RunStatusActive, runs[0].Status)

	// Second tick: still active and now due → fire.
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 1)
}

func TestRunner_DeliveryDuplicateIdempotence(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Pre-insert a delivery row to simulate a pre-crash reservation.
	chID := "1"
	_, err := h.store.InsertDelivery(context.Background(), &Delivery{
		RunID:            h.store.onlyRunID(),
		LevelIndex:       0,
		ChannelID:        &chID,
		Status:           DeliveryStatusSent, // already done before crash
		AttemptStartedAt: h.now,
	})
	require.NoError(t, err)

	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	// No new delivery dispatched — duplicate caught silently.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), h.sender.delivers.Load())
	// Run still advances (level is considered done).
	runs := h.store.listRuns()
	require.Len(t, runs, 1)
	assert.Equal(t, 0, runs[0].LastExecutedLevelIndex)
}

func TestRunner_OneChannelFailureDoesNotBlockOthers(t *testing.T) {
	h := newHarness(t)
	h.channels.put(&alert.NotificationChannel{ID: "1", Name: "good", Type: "slack", URL: "u1", Enabled: true})
	h.channels.put(&alert.NotificationChannel{ID: "2", Name: "bad", Type: "slack", URL: "u2", Enabled: true})
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(&Policy{
		Name: "p", Active: true,
		Filters: Filters{Severities: []string{alert.SeverityCritical}},
		Levels:  []Level{{Order: 0, DelaySeconds: 60, ChannelIDs: []string{"1", "2"}}},
	})
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	h.sender.failFor["2"] = true
	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 2)

	deliveries := h.store.listDeliveries()
	require.Len(t, deliveries, 2)
	statuses := map[string]string{}
	for _, d := range deliveries {
		require.NotNil(t, d.ChannelID)
		statuses[*d.ChannelID] = d.Status
	}
	// Wait for async UpdateDelivery to land.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, d := range h.store.listDeliveries() {
			if d.Status == DeliveryStatusPending {
				ok = false
				break
			}
		}
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, d := range h.store.listDeliveries() {
		require.NotNil(t, d.ChannelID)
		statuses[*d.ChannelID] = d.Status
	}
	assert.Equal(t, DeliveryStatusSent, statuses["1"])
	assert.Equal(t, DeliveryStatusFailed, statuses["2"])
}

func TestRunner_DisabledChannelMarksDeliveryFailed(t *testing.T) {
	h := newHarness(t)
	h.channels.put(&alert.NotificationChannel{ID: "1", Name: "off", Type: "slack", URL: "u", Enabled: false})
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	h.advance(61 * time.Second)
	require.NoError(t, h.runner.EvaluateCycle(context.Background()))

	// No network call (channel disabled).
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), h.sender.delivers.Load())

	deliveries := h.store.listDeliveries()
	require.Len(t, deliveries, 1)
	assert.Equal(t, DeliveryStatusFailed, deliveries[0].Status)
	assert.Contains(t, deliveries[0].Error, "disabled")
}

func TestRunner_OrphanRecoveryRetries(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Manually insert a stale pending delivery (simulating a crash mid-send).
	stale := h.now.Add(-5 * time.Minute)
	chID := "1"
	_, err := h.store.InsertDelivery(context.Background(), &Delivery{
		RunID:            h.store.onlyRunID(),
		LevelIndex:       0,
		ChannelID:        &chID,
		Status:           DeliveryStatusPending,
		AttemptStartedAt: stale,
	})
	require.NoError(t, err)

	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	h.waitForDeliveries(t, 1)

	deliveries := h.store.listDeliveries()
	require.Len(t, deliveries, 1)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && deliveries[0].Status == DeliveryStatusPending {
		time.Sleep(5 * time.Millisecond)
		deliveries = h.store.listDeliveries()
	}
	assert.Equal(t, DeliveryStatusSent, deliveries[0].Status)
}

func TestRunner_OrphanAbandonedWhenAlertResolved(t *testing.T) {
	h := newHarness(t)
	h.channels.put(defaultChannel())
	a := criticalAlert("1")
	h.alerts.put(a)
	h.store.addPolicy(policyTwoLevels())
	require.NoError(t, h.runner.OnAlertCreated(context.Background(), a))

	// Stale pending row.
	chID := "1"
	_, err := h.store.InsertDelivery(context.Background(), &Delivery{
		RunID:            h.store.onlyRunID(),
		LevelIndex:       0,
		ChannelID:        &chID,
		Status:           DeliveryStatusPending,
		AttemptStartedAt: h.now.Add(-5 * time.Minute),
	})
	require.NoError(t, err)

	// Resolve the alert.
	a.Status = alert.StatusResolved
	h.alerts.put(a)

	require.NoError(t, h.runner.EvaluateCycle(context.Background()))
	deliveries := h.store.listDeliveries()
	require.Len(t, deliveries, 1)
	assert.Equal(t, DeliveryStatusAbandoned, deliveries[0].Status)
	assert.Equal(t, int32(0), h.sender.delivers.Load())
}

func TestRunner_OnEditionDowngradedDelegates(t *testing.T) {
	h := newHarness(t)
	// Service.OnEditionDowngraded calls store.BulkDeactivateAllPolicies and
	// store.BulkStopActiveRuns. Our mock returns nil for both — no-op success
	// path validates the delegation chain.
	require.NoError(t, h.runner.OnEditionDowngraded(context.Background()))
}
