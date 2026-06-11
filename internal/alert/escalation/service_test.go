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
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- store mock ---

type mockStore struct {
	policies    map[string]*Policy
	activeCount int
	insertErr   error
	selectErr   error
}

func newMockStore() *mockStore {
	return &mockStore{policies: map[string]*Policy{}}
}

func (m *mockStore) InsertPolicy(_ context.Context, p *Policy) (string, error) {
	if m.insertErr != nil {
		return "", m.insertErr
	}
	p.ID = uid.New()
	m.policies[p.ID] = p
	if p.Active {
		m.activeCount++
	}
	return p.ID, nil
}
func (m *mockStore) UpdatePolicy(_ context.Context, p *Policy) error {
	m.policies[p.ID] = p
	return nil
}
func (m *mockStore) SelectPolicy(_ context.Context, id string) (*Policy, error) {
	if m.selectErr != nil {
		return nil, m.selectErr
	}
	return m.policies[id], nil
}
func (m *mockStore) SelectPolicies(_ context.Context, activeOnly bool) ([]*Policy, error) {
	var out []*Policy
	for _, p := range m.policies {
		if !activeOnly || p.Active {
			out = append(out, p)
		}
	}
	return out, nil
}
func (m *mockStore) DeletePolicy(_ context.Context, id string) error {
	p, ok := m.policies[id]
	if ok && p.Active {
		m.activeCount--
	}
	delete(m.policies, id)
	return nil
}
func (m *mockStore) CountActivePolicies(_ context.Context) (int, error)  { return m.activeCount, nil }
func (m *mockStore) SelectRun(_ context.Context, _ string) (*Run, error) { return nil, nil }
func (m *mockStore) SelectRunsByAlert(_ context.Context, _ string) ([]*Run, error) {
	return []*Run{}, nil
}
func (m *mockStore) SelectRunsByPolicy(_ context.Context, _ string, _ int, _ string) ([]*Run, error) {
	return []*Run{}, nil
}
func (m *mockStore) SelectRunDeliveries(_ context.Context, _ string) ([]*Delivery, error) {
	return []*Delivery{}, nil
}
func (m *mockStore) BulkDeactivateAllPolicies(_ context.Context) error        { return nil }
func (m *mockStore) BulkRestorePoliciesFromDowngrade(_ context.Context) error { return nil }
func (m *mockStore) BulkStopActiveRuns(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, _ time.Time) error {
	return nil
}
func (m *mockStore) InsertRun(_ context.Context, _ *Run) (string, error) { return "", nil }
func (m *mockStore) UpdateRunProgress(_ context.Context, _ string, _ int, _ *time.Time, _ string) error {
	return nil
}
func (m *mockStore) TerminateRun(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (m *mockStore) SelectActiveRunsByAlert(_ context.Context, _ string) ([]*Run, error) {
	return nil, nil
}
func (m *mockStore) SelectDueRuns(_ context.Context, _ time.Time) ([]*Run, error) {
	return nil, nil
}
func (m *mockStore) PauseRunForMaintenance(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockStore) ResumeRunFromMaintenance(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockStore) InsertDelivery(_ context.Context, _ *Delivery) (string, error) { return "", nil }
func (m *mockStore) UpdateDelivery(_ context.Context, _ *Delivery) error           { return nil }
func (m *mockStore) SelectOrphanPendingDeliveries(_ context.Context, _ time.Time) ([]*Delivery, error) {
	return nil, nil
}

// --- channel store mock ---

type mockChannelStore struct{}

func (m *mockChannelStore) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (string, error) {
	return "1", nil
}
func (m *mockChannelStore) GetChannel(_ context.Context, _ string) (*alert.NotificationChannel, error) {
	return &alert.NotificationChannel{ID: "1", Name: "test", Enabled: true}, nil
}
func (m *mockChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *mockChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (m *mockChannelStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (m *mockChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (m *mockChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "1", nil
}
func (m *mockChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *mockChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// --- suppressor mock ---

type mockSuppressor struct{ suppressed bool }

func (m *mockSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	return m.suppressed, nil
}

// --- helpers ---

func newTestService(store *mockStore) *Service {
	return NewService(
		store,
		&mockChannelStore{},
		func() extension.Edition { return extension.Pro },
		&mockSuppressor{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	)
}

func validRequest() PolicyRequest {
	return PolicyRequest{
		Name:   "test policy",
		Active: false,
		Filters: Filters{
			Severities: []string{"critical"},
			Scopes:     []Scope{},
			Tags:       []string{},
		},
		Levels: []LevelReq{
			{DelaySeconds: 300, ChannelIDs: []string{"1"}},
		},
	}
}

// --- tests ---

func TestCreatePolicy_EmptyName(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Name = ""
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_NameTooLong(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Name = string(make([]byte, 121))
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_NoLevels(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{}
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_DelayTooShort(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{{DelaySeconds: 30, ChannelIDs: []string{"1"}}}
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_DelayTooLong(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{{DelaySeconds: 90000, ChannelIDs: []string{"1"}}}
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_EmptyChannelIDs(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{{DelaySeconds: 300, ChannelIDs: []string{}}}
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_IntervalTooShort(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{
		{DelaySeconds: 300, ChannelIDs: []string{"1"}},
		{DelaySeconds: 330, ChannelIDs: []string{"1"}}, // only 30s gap < 60s
	}
	_, err := svc.CreatePolicy(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidationFailed))
}

func TestCreatePolicy_TwoToFiveLevels_OK(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	req.Levels = []LevelReq{
		{DelaySeconds: 300, ChannelIDs: []string{"1"}},
		{DelaySeconds: 600, ChannelIDs: []string{"1"}},
		{DelaySeconds: 900, ChannelIDs: []string{"1"}},
	}
	p, err := svc.CreatePolicy(context.Background(), req)
	require.NoError(t, err)
	assert.Len(t, p.Levels, 3)
	assert.Equal(t, 0, p.Levels[0].Order)
	assert.Equal(t, 2, p.Levels[2].Order)
}

func TestCreatePolicy_HappyPath(t *testing.T) {
	svc := newTestService(newMockStore())
	req := validRequest()
	p, err := svc.CreatePolicy(context.Background(), req)
	require.NoError(t, err)
	assert.NotZero(t, p.ID)
	assert.Equal(t, "test policy", p.Name)
	assert.Equal(t, 0, p.Levels[0].Order)
}

func TestGetPolicy_NotFound(t *testing.T) {
	svc := newTestService(newMockStore())
	_, err := svc.GetPolicy(context.Background(), "999")
	assert.True(t, errors.Is(err, ErrPolicyNotFound))
}

func TestGetPolicy_Found(t *testing.T) {
	store := newMockStore()
	svc := newTestService(store)
	req := validRequest()
	created, err := svc.CreatePolicy(context.Background(), req)
	require.NoError(t, err)

	p, err := svc.GetPolicy(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, p.ID)
}

func TestDeletePolicy_NotFound(t *testing.T) {
	svc := newTestService(newMockStore())
	err := svc.DeletePolicy(context.Background(), "999")
	assert.True(t, errors.Is(err, ErrPolicyNotFound))
}

func TestDeletePolicy_HappyPath(t *testing.T) {
	store := newMockStore()
	svc := newTestService(store)
	req := validRequest()
	created, err := svc.CreatePolicy(context.Background(), req)
	require.NoError(t, err)

	err = svc.DeletePolicy(context.Background(), created.ID)
	require.NoError(t, err)

	_, err = svc.GetPolicy(context.Background(), created.ID)
	assert.True(t, errors.Is(err, ErrPolicyNotFound))
}

func TestIsAlertSuppressed(t *testing.T) {
	svc := NewService(
		newMockStore(),
		&mockChannelStore{},
		func() extension.Edition { return extension.Pro },
		&mockSuppressor{suppressed: true},
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
	)
	suppressed, err := svc.IsAlertSuppressed(context.Background(), "42")
	require.NoError(t, err)
	assert.True(t, suppressed)
}

func TestIsAlertSuppressed_NotSuppressed(t *testing.T) {
	svc := NewService(
		newMockStore(),
		&mockChannelStore{},
		func() extension.Edition { return extension.Pro },
		&mockSuppressor{suppressed: false},
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
	)
	suppressed, err := svc.IsAlertSuppressed(context.Background(), "99")
	require.NoError(t, err)
	assert.False(t, suppressed)
}

func TestUpdatePolicy_HappyPath(t *testing.T) {
	store := newMockStore()
	svc := newTestService(store)

	created, err := svc.CreatePolicy(context.Background(), validRequest())
	require.NoError(t, err)

	req := PolicyRequest{
		Name:   "updated name",
		Active: false,
		Filters: Filters{
			Severities: []string{"warning"},
			Scopes:     []Scope{},
			Tags:       []string{},
		},
		Levels: []LevelReq{
			{DelaySeconds: 300, ChannelIDs: []string{"1"}},
			{DelaySeconds: 600, ChannelIDs: []string{"1"}},
		},
	}
	p, err := svc.UpdatePolicy(context.Background(), created.ID, req)
	require.NoError(t, err)
	assert.Equal(t, "updated name", p.Name)
	assert.Len(t, p.Levels, 2)
}

func TestUpdatePolicy_NotFound(t *testing.T) {
	svc := newTestService(newMockStore())
	_, err := svc.UpdatePolicy(context.Background(), "999", validRequest())
	assert.True(t, errors.Is(err, ErrPolicyNotFound))
}

func TestSetPolicyActive_HappyPath(t *testing.T) {
	store := newMockStore()
	svc := newTestService(store)

	req := validRequest()
	req.Active = false
	created, err := svc.CreatePolicy(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, created.Active)

	p, err := svc.SetPolicyActive(context.Background(), created.ID, true)
	require.NoError(t, err)
	assert.True(t, p.Active)
}

func TestSetPolicyActive_NotFound(t *testing.T) {
	svc := newTestService(newMockStore())
	_, err := svc.SetPolicyActive(context.Background(), "999", true)
	assert.True(t, errors.Is(err, ErrPolicyNotFound))
}
