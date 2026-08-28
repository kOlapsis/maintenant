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

package mcp

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock store for MCP tests ---

type mcpEscalationStore struct {
	policies    map[string]*escalation.Policy
	nextID      int64
	activeCount int
}

func newMCPEscalationStore() *mcpEscalationStore {
	return &mcpEscalationStore{policies: map[string]*escalation.Policy{}}
}

func (m *mcpEscalationStore) InsertPolicy(_ context.Context, p *escalation.Policy) (string, error) {
	m.nextID++
	p.ID = strconv.FormatInt(m.nextID, 10)
	cp := *p
	m.policies[p.ID] = &cp
	if p.Active {
		m.activeCount++
	}
	return p.ID, nil
}
func (m *mcpEscalationStore) UpdatePolicy(_ context.Context, p *escalation.Policy) error {
	m.policies[p.ID] = p
	return nil
}
func (m *mcpEscalationStore) SelectPolicy(_ context.Context, id string) (*escalation.Policy, error) {
	return m.policies[id], nil
}
func (m *mcpEscalationStore) SelectPolicies(_ context.Context, _ bool) ([]*escalation.Policy, error) {
	var out []*escalation.Policy
	for _, p := range m.policies {
		out = append(out, p)
	}
	if out == nil {
		return []*escalation.Policy{}, nil
	}
	return out, nil
}
func (m *mcpEscalationStore) DeletePolicy(_ context.Context, id string) error {
	if p, ok := m.policies[id]; ok && p.Active {
		m.activeCount--
	}
	delete(m.policies, id)
	return nil
}
func (m *mcpEscalationStore) CountActivePolicies(_ context.Context) (int, error) {
	return m.activeCount, nil
}
func (m *mcpEscalationStore) SelectRun(_ context.Context, _ string) (*escalation.Run, error) {
	return nil, nil
}
func (m *mcpEscalationStore) SelectRunsByAlert(_ context.Context, _ string) ([]*escalation.Run, error) {
	return []*escalation.Run{}, nil
}
func (m *mcpEscalationStore) SelectRunsByPolicy(_ context.Context, _ string, _ int, _ string) ([]*escalation.Run, error) {
	return []*escalation.Run{}, nil
}
func (m *mcpEscalationStore) SelectRunDeliveries(_ context.Context, _ string) ([]*escalation.Delivery, error) {
	return []*escalation.Delivery{}, nil
}
func (m *mcpEscalationStore) BulkDeactivateAllPolicies(_ context.Context) error        { return nil }
func (m *mcpEscalationStore) BulkRestorePoliciesFromDowngrade(_ context.Context) error { return nil }
func (m *mcpEscalationStore) BulkStopActiveRuns(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mcpEscalationStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, _ time.Time) error {
	return nil
}
func (m *mcpEscalationStore) InsertRun(_ context.Context, _ *escalation.Run) (string, error) {
	return "", nil
}
func (m *mcpEscalationStore) UpdateRunProgress(_ context.Context, _ string, _ int, _ *time.Time, _ string) error {
	return nil
}
func (m *mcpEscalationStore) TerminateRun(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (m *mcpEscalationStore) SelectActiveRunsByAlert(_ context.Context, _ string) ([]*escalation.Run, error) {
	return nil, nil
}
func (m *mcpEscalationStore) SelectDueRuns(_ context.Context, _ time.Time) ([]*escalation.Run, error) {
	return nil, nil
}
func (m *mcpEscalationStore) PauseRunForMaintenance(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mcpEscalationStore) ResumeRunFromMaintenance(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mcpEscalationStore) InsertDelivery(_ context.Context, _ *escalation.Delivery) (string, error) {
	return "", nil
}
func (m *mcpEscalationStore) UpdateDelivery(_ context.Context, _ *escalation.Delivery) error {
	return nil
}
func (m *mcpEscalationStore) SelectOrphanPendingDeliveries(_ context.Context, _ time.Time) ([]*escalation.Delivery, error) {
	return nil, nil
}

// --- channel store mock ---

type mcpChannelStore struct{}

func (m *mcpChannelStore) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (string, error) {
	return "1", nil
}
func (m *mcpChannelStore) GetChannel(_ context.Context, id string) (*alert.NotificationChannel, error) {
	return &alert.NotificationChannel{ID: id, Name: "chan", Enabled: true}, nil
}
func (m *mcpChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *mcpChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (m *mcpChannelStore) DeleteChannel(_ context.Context, _ string) error { return nil }
func (m *mcpChannelStore) GetChannelHealth(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
func (m *mcpChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (string, error) {
	return "1", nil
}
func (m *mcpChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *mcpChannelStore) ListDeliveriesByAlert(_ context.Context, _ string) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// --- suppressor mock ---

type mcpNoopSuppressor struct{}

func (mcpNoopSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// --- helpers ---

func buildProEscalationServices() *Services {
	svc := escalation.NewService(
		newMCPEscalationStore(),
		&mcpChannelStore{},
		func() extension.Edition { return extension.Pro },
		mcpNoopSuppressor{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	)
	return &Services{EscalationSvc: svc, Logger: slog.Default(), Version: "test"}
}

func buildCEEscalationServices() *Services {
	return &Services{EscalationSvc: nil, Logger: slog.Default(), Version: "test"}
}

// --- tests: CE gating returns edition_required ---

func TestEscalationTools_CE_ListPolicies(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := listEscalationPoliciesHandler(svc)
	result, _, err := handler(context.Background(), nil, listEscalationPoliciesInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "edition_required")
}

func TestEscalationTools_CE_GetPolicy(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := getEscalationPolicyHandler(svc)
	result, _, err := handler(context.Background(), nil, getEscalationPolicyInput{ID: "1"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_CE_CreatePolicy(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := createEscalationPolicyHandler(svc)
	result, _, err := handler(context.Background(), nil, createEscalationPolicyInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_CE_DeletePolicy(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := deleteEscalationPolicyHandler(svc)
	result, _, err := handler(context.Background(), nil, deleteEscalationPolicyInput{ID: "1"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_CE_ListAlertRuns(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := listAlertEscalationRunsHandler(svc)
	result, _, err := handler(context.Background(), nil, listAlertEscalationRunsInput{AlertID: "1"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_CE_GetRun(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := getEscalationRunHandler(svc)
	result, _, err := handler(context.Background(), nil, getEscalationRunInput{ID: "1"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

// --- tests: Pro happy path ---

func TestEscalationTools_Pro_ListPolicies(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()
	handler := listEscalationPoliciesHandler(svc)
	result, _, err := handler(context.Background(), nil, listEscalationPoliciesInput{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestEscalationTools_Pro_CreateAndGetPolicy(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()
	createHandler := createEscalationPolicyHandler(svc)
	result, _, err := createHandler(context.Background(), nil, createEscalationPolicyInput{
		Name:   "mcp test policy",
		Active: false,
		Levels: []escalationLevelInput{
			{DelaySeconds: 300, ChannelIDs: []string{"1"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "mcp test policy")
	assert.Contains(t, text, "overlapping_warnings")
}

func TestEscalationTools_Pro_DeletePolicy_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()
	handler := deleteEscalationPolicyHandler(svc)
	result, _, err := handler(context.Background(), nil, deleteEscalationPolicyInput{ID: "999"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "policy_not_found")
}

func TestEscalationTools_CE_UpdatePolicy(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := updateEscalationPolicyHandler(svc)
	result, _, err := handler(context.Background(), nil, updateEscalationPolicyInput{ID: "1"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_CE_SetPolicyActive(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	svc := buildCEEscalationServices()
	handler := setEscalationPolicyActiveHandler(svc)
	result, _, err := handler(context.Background(), nil, setEscalationPolicyActiveInput{ID: "1", Active: true})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestEscalationTools_Pro_SetPolicyActive_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()

	// Create an inactive policy first
	createHandler := createEscalationPolicyHandler(svc)
	createResult, _, err := createHandler(context.Background(), nil, createEscalationPolicyInput{
		Name:   "policy to activate",
		Active: false,
		Levels: []escalationLevelInput{{DelaySeconds: 300, ChannelIDs: []string{"1"}}},
	})
	require.NoError(t, err)
	require.False(t, createResult.IsError)

	// Activate it
	handler := setEscalationPolicyActiveHandler(svc)
	result, _, err := handler(context.Background(), nil, setEscalationPolicyActiveInput{ID: "1", Active: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, "true")
}

func TestEscalationTools_Pro_UpdatePolicy_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()

	// First create a policy
	createHandler := createEscalationPolicyHandler(svc)
	createResult, _, err := createHandler(context.Background(), nil, createEscalationPolicyInput{
		Name:   "original name",
		Active: false,
		Levels: []escalationLevelInput{{DelaySeconds: 300, ChannelIDs: []string{"1"}}},
	})
	require.NoError(t, err)
	require.False(t, createResult.IsError)

	// Update it
	updateHandler := updateEscalationPolicyHandler(svc)
	result, _, err := updateHandler(context.Background(), nil, updateEscalationPolicyInput{
		ID:     "1",
		Name:   "updated name",
		Active: false,
		Levels: []escalationLevelInput{
			{DelaySeconds: 300, ChannelIDs: []string{"1"}},
			{DelaySeconds: 600, ChannelIDs: []string{"1"}},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "updated name")
}

func TestEscalationTools_Pro_ListAlertRuns_Empty(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()
	handler := listAlertEscalationRunsHandler(svc)
	result, _, err := handler(context.Background(), nil, listAlertEscalationRunsInput{AlertID: "42"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "runs")
}

func TestEscalationTools_Pro_GetRun_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	svc := buildProEscalationServices()
	handler := getEscalationRunHandler(svc)
	result, _, err := handler(context.Background(), nil, getEscalationRunInput{ID: "999"})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "run_not_found")
}
