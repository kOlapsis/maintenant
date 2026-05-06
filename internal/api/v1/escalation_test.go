// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- in-memory mock store for escalation ---

type escalationTestStore struct {
	policies     map[int64]*escalation.Policy
	nextID       int64
	activeCount  int
}

func newEscalationTestStore() *escalationTestStore {
	return &escalationTestStore{policies: map[int64]*escalation.Policy{}}
}

func (m *escalationTestStore) InsertPolicy(_ context.Context, p *escalation.Policy) (int64, error) {
	m.nextID++
	p.ID = m.nextID
	cp := *p
	m.policies[p.ID] = &cp
	if p.Active {
		m.activeCount++
	}
	return p.ID, nil
}
func (m *escalationTestStore) UpdatePolicy(_ context.Context, p *escalation.Policy) error {
	m.policies[p.ID] = p
	return nil
}
func (m *escalationTestStore) SelectPolicy(_ context.Context, id int64) (*escalation.Policy, error) {
	return m.policies[id], nil
}
func (m *escalationTestStore) SelectPolicies(_ context.Context, activeOnly bool) ([]*escalation.Policy, error) {
	var out []*escalation.Policy
	for _, p := range m.policies {
		if !activeOnly || p.Active {
			out = append(out, p)
		}
	}
	if out == nil {
		return []*escalation.Policy{}, nil
	}
	return out, nil
}
func (m *escalationTestStore) DeletePolicy(_ context.Context, id int64) error {
	if p, ok := m.policies[id]; ok && p.Active {
		m.activeCount--
	}
	delete(m.policies, id)
	return nil
}
func (m *escalationTestStore) CountActivePolicies(_ context.Context) (int, error) {
	return m.activeCount, nil
}
func (m *escalationTestStore) SelectRun(_ context.Context, _ int64) (*escalation.Run, error) {
	return nil, nil
}
func (m *escalationTestStore) SelectRunsByAlert(_ context.Context, _ int64) ([]*escalation.Run, error) {
	return []*escalation.Run{}, nil
}
func (m *escalationTestStore) SelectRunsByPolicy(_ context.Context, _ int64, _ int, _ int64) ([]*escalation.Run, error) {
	return []*escalation.Run{}, nil
}
func (m *escalationTestStore) SelectRunDeliveries(_ context.Context, _ int64) ([]*escalation.Delivery, error) {
	return []*escalation.Delivery{}, nil
}
func (m *escalationTestStore) BulkDeactivateAllPolicies(_ context.Context) error       { return nil }
func (m *escalationTestStore) BulkRestorePoliciesFromDowngrade(_ context.Context) error { return nil }
func (m *escalationTestStore) BulkStopActiveRuns(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *escalationTestStore) PurgeRunsAndDeliveriesOlderThan(_ context.Context, _ time.Time) error {
	return nil
}

// --- in-memory channel store mock ---

type escalationTestChannelStore struct{}

func (m *escalationTestChannelStore) InsertChannel(_ context.Context, _ *alert.NotificationChannel) (int64, error) {
	return 1, nil
}
func (m *escalationTestChannelStore) GetChannel(_ context.Context, id int64) (*alert.NotificationChannel, error) {
	return &alert.NotificationChannel{ID: id, Name: "test-channel", Enabled: true}, nil
}
func (m *escalationTestChannelStore) ListChannels(_ context.Context) ([]*alert.NotificationChannel, error) {
	return nil, nil
}
func (m *escalationTestChannelStore) UpdateChannel(_ context.Context, _ *alert.NotificationChannel) error {
	return nil
}
func (m *escalationTestChannelStore) DeleteChannel(_ context.Context, _ int64) error { return nil }
func (m *escalationTestChannelStore) GetChannelHealth(_ context.Context, _ int64) (string, error) {
	return "ok", nil
}
func (m *escalationTestChannelStore) InsertRoutingRule(_ context.Context, _ *alert.RoutingRule) (int64, error) {
	return 1, nil
}
func (m *escalationTestChannelStore) DeleteRoutingRule(_ context.Context, _ int64) error { return nil }
func (m *escalationTestChannelStore) ListRoutingRulesByChannel(_ context.Context, _ int64) ([]alert.RoutingRule, error) {
	return nil, nil
}
func (m *escalationTestChannelStore) InsertDelivery(_ context.Context, _ *alert.NotificationDelivery) (int64, error) {
	return 1, nil
}
func (m *escalationTestChannelStore) UpdateDelivery(_ context.Context, _ *alert.NotificationDelivery) error {
	return nil
}
func (m *escalationTestChannelStore) ListDeliveriesByAlert(_ context.Context, _ int64) ([]*alert.NotificationDelivery, error) {
	return nil, nil
}

// --- suppressor mock ---

type noopSuppressor struct{}

func (n noopSuppressor) IsSuppressed(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// --- helpers ---

func buildEscalationHandler(tier extension.PlanTier) *EscalationHandler {
	svc := escalation.NewService(
		newEscalationTestStore(),
		&escalationTestChannelStore{},
		func() extension.Edition { return extension.Enterprise },
		func() extension.PlanTier { return tier },
		noopSuppressor{},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	)
	return NewEscalationHandler(svc)
}

func escalationMux(t *testing.T, tier extension.PlanTier) *http.ServeMux {
	t.Helper()
	h := buildEscalationHandler(tier)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/escalation-policies", requireEnterprise(h.HandleCreatePolicy))
	mux.HandleFunc("GET /api/v1/escalation-policies", requireEnterprise(h.HandleListPolicies))
	mux.HandleFunc("POST /api/v1/escalation-policies/overlap-probe", requireEnterprise(h.HandleOverlapProbe))
	mux.HandleFunc("GET /api/v1/escalation-policies/{id}", requireEnterprise(h.HandleGetPolicy))
	mux.HandleFunc("DELETE /api/v1/escalation-policies/{id}", requireEnterprise(h.HandleDeletePolicy))
	mux.HandleFunc("PUT /api/v1/escalation-policies/{id}", requireEnterprise(h.HandleUpdatePolicy))
	mux.HandleFunc("PATCH /api/v1/escalation-policies/{id}/active", requireEnterprise(h.HandleSetPolicyActive))
	mux.HandleFunc("GET /api/v1/escalation-policies/{id}/runs", requireEnterprise(h.HandleListPolicyRuns))
	mux.HandleFunc("GET /api/v1/escalation-runs/{run_id}", requireEnterprise(h.HandleGetRun))
	mux.HandleFunc("GET /api/v1/alerts/{alert_id}/escalation-runs", requireEnterprise(h.HandleListAlertRuns))
	return mux
}

func validPolicyBody() escalation.PolicyRequest {
	return escalation.PolicyRequest{
		Name:   "test policy",
		Active: false,
		Filters: escalation.Filters{
			Severities: []string{"critical"},
			Scopes:     []escalation.Scope{},
			Tags:       []string{},
		},
		Levels: []escalation.LevelReq{
			{DelaySeconds: 300, ChannelIDs: []int64{1}},
		},
	}
}

// --- tests: CE gating ---

func TestEscalation_AllEndpoints_Return403_CE(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/escalation-policies"},
		{"GET", "/api/v1/escalation-policies"},
		{"GET", "/api/v1/escalation-policies/1"},
		{"DELETE", "/api/v1/escalation-policies/1"},
	}

	mux := escalationMux(t, extension.PlanTierTeam)
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(ep.method, ep.path, nil)
			mux.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

// --- tests: Pro happy path ---

func TestEscalation_CreatePolicy_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	b, _ := json.Marshal(validPolicyBody())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var policy escalation.Policy
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&policy))
	assert.NotZero(t, policy.ID)
	assert.Equal(t, "test policy", policy.Name)
}

func TestEscalation_CreatePolicy_ValidationError_EmptyName(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	body := validPolicyBody()
	body.Name = ""
	b, _ := json.Marshal(body)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEscalation_ListPolicies_Empty(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/escalation-policies", nil)
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Policies []escalation.Policy `json:"policies"`
		Limits   escalation.Limits   `json:"limits"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp.Policies)
	assert.Equal(t, 25, resp.Limits.MaxActive)
}

func TestEscalation_GetPolicy_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/escalation-policies/999", nil)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEscalation_DeletePolicy_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/escalation-policies/999", nil)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEscalation_DeletePolicy_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)

	// Create first
	b, _ := json.Marshal(validPolicyBody())
	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	reqCreate.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created escalation.Policy
	require.NoError(t, json.NewDecoder(recCreate.Body).Decode(&created))

	// Delete
	recDel := httptest.NewRecorder()
	reqDel := httptest.NewRequest("DELETE", "/api/v1/escalation-policies/"+itoa(int(created.ID)), nil)
	mux.ServeHTTP(recDel, reqDel)
	assert.Equal(t, http.StatusNoContent, recDel.Code)
}

func TestEscalation_OverlapProbe_ReturnsEmpty(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/escalation-policies/overlap-probe", nil)
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	overlapping, ok := resp["overlapping"].([]any)
	require.True(t, ok)
	assert.Empty(t, overlapping)
}

func TestEscalation_UpdatePolicy_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)

	// Create first
	b, _ := json.Marshal(validPolicyBody())
	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	reqCreate.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created escalation.Policy
	require.NoError(t, json.NewDecoder(recCreate.Body).Decode(&created))

	// Update with new name and 2 levels
	updateBody := escalation.PolicyRequest{
		Name:   "updated policy",
		Active: false,
		Filters: escalation.Filters{
			Severities: []string{"critical"},
			Scopes:     []escalation.Scope{},
			Tags:       []string{},
		},
		Levels: []escalation.LevelReq{
			{DelaySeconds: 300, ChannelIDs: []int64{1}},
			{DelaySeconds: 600, ChannelIDs: []int64{1}},
		},
	}
	bUpdate, _ := json.Marshal(updateBody)
	recUpdate := httptest.NewRecorder()
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/escalation-policies/"+itoa(int(created.ID)), bytes.NewReader(bUpdate))
	reqUpdate.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recUpdate, reqUpdate)

	require.Equal(t, http.StatusOK, recUpdate.Code)
	var updated escalation.Policy
	require.NoError(t, json.NewDecoder(recUpdate.Body).Decode(&updated))
	assert.Equal(t, "updated policy", updated.Name)
	assert.Len(t, updated.Levels, 2)
}

func TestEscalation_UpdatePolicy_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	b, _ := json.Marshal(validPolicyBody())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/escalation-policies/999", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEscalation_UpdatePolicy_ValidationError(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)

	// Create first
	b, _ := json.Marshal(validPolicyBody())
	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	reqCreate.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created escalation.Policy
	require.NoError(t, json.NewDecoder(recCreate.Body).Decode(&created))

	// Update with empty name
	invalidBody := validPolicyBody()
	invalidBody.Name = ""
	bUpdate, _ := json.Marshal(invalidBody)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/escalation-policies/"+itoa(int(created.ID)), bytes.NewReader(bUpdate))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEscalation_SetPolicyActive_HappyPath(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)

	// Create inactive policy
	b, _ := json.Marshal(validPolicyBody())
	recCreate := httptest.NewRecorder()
	reqCreate := httptest.NewRequest("POST", "/api/v1/escalation-policies", bytes.NewReader(b))
	reqCreate.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var created escalation.Policy
	require.NoError(t, json.NewDecoder(recCreate.Body).Decode(&created))

	// Activate
	patchBody, _ := json.Marshal(map[string]bool{"active": true})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/escalation-policies/"+itoa(int(created.ID))+"/active", bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, true, resp["active"])
}

func TestEscalation_SetPolicyActive_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	patchBody, _ := json.Marshal(map[string]bool{"active": true})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/escalation-policies/999/active", bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEscalation_GetAlertRuns_Pro_Empty(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/alerts/1/escalation-runs", nil)
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Runs []any `json:"runs"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp.Runs)
}

func TestEscalation_GetAlertRuns_CE_403(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/alerts/1/escalation-runs", nil)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestEscalation_GetRun_NotFound(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/escalation-runs/999", nil)
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEscalation_ListPolicyRuns_Pro_Empty(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Enterprise }
	defer func() { extension.CurrentEdition = original }()

	mux := escalationMux(t, extension.PlanTierTeam)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/escalation-policies/1/runs", nil)
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Runs []any `json:"runs"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp.Runs)
}

// itoa converts an int to string without fmt overhead.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}
