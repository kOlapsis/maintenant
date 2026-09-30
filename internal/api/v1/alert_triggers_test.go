// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stub trigger store
// ---------------------------------------------------------------------------

type stubTriggerStore struct {
	triggers  map[string]*alert.AlertTrigger
	nextID    int64
	insertErr error
}

func newStubTriggerStore() *stubTriggerStore {
	return &stubTriggerStore{triggers: map[string]*alert.AlertTrigger{}}
}

func (s *stubTriggerStore) InsertTrigger(_ context.Context, t *alert.AlertTrigger) (string, error) {
	if s.insertErr != nil {
		return "", s.insertErr
	}
	s.nextID++
	t.ID = fmt.Sprintf("%d", s.nextID)
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	cp := *t
	s.triggers[t.ID] = &cp
	return t.ID, nil
}
func (s *stubTriggerStore) GetTrigger(_ context.Context, id string) (*alert.AlertTrigger, error) {
	return s.triggers[id], nil
}
func (s *stubTriggerStore) ListTriggers(_ context.Context) ([]*alert.AlertTrigger, error) {
	out := make([]*alert.AlertTrigger, 0, len(s.triggers))
	for _, t := range s.triggers {
		out = append(out, t)
	}
	return out, nil
}
func (s *stubTriggerStore) ListEnabledTriggers(_ context.Context) ([]*alert.AlertTrigger, error) {
	return nil, nil
}
func (s *stubTriggerStore) UpdateTrigger(_ context.Context, t *alert.AlertTrigger) error {
	if _, ok := s.triggers[t.ID]; !ok {
		return fmt.Errorf("not found")
	}
	cp := *t
	s.triggers[t.ID] = &cp
	return nil
}
func (s *stubTriggerStore) DeleteTrigger(_ context.Context, id string) error {
	delete(s.triggers, id)
	return nil
}
func (s *stubTriggerStore) SetChannels(_ context.Context, _ string, _ []string) error { return nil }
func (s *stubTriggerStore) ListChannelsForTrigger(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}
func (s *stubTriggerStore) ListTriggersForChannel(_ context.Context, _ string) ([]*alert.AlertTrigger, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTriggerHandler(channelExists bool) (*AlertTriggerHandler, *stubTriggerStore) {
	ts := newStubTriggerStore()
	var ch *alert.NotificationChannel
	if channelExists {
		ch = &alert.NotificationChannel{ID: "1", Name: "test-ch", Enabled: true}
	}
	cs := &stubChannelStore{ch: ch}
	return NewAlertTriggerHandler(ts, cs, nil), ts
}

func seedTrigger(t *testing.T, ts *stubTriggerStore, name string) string {
	t.Helper()
	trig := &alert.AlertTrigger{Name: name, Enabled: true, ChannelIDs: []string{"1"}}
	id, err := ts.InsertTrigger(context.Background(), trig)
	require.NoError(t, err)
	return id
}

// ---------------------------------------------------------------------------
// HandleListTriggers
// ---------------------------------------------------------------------------

func TestHandleListTriggers_Empty(t *testing.T) {
	h, _ := newTriggerHandler(true)
	req := httptest.NewRequest("GET", "/api/v1/alert-triggers", nil)
	rec := httptest.NewRecorder()
	h.HandleListTriggers(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"triggers"`)
}

func TestHandleListTriggers_WithData(t *testing.T) {
	h, ts := newTriggerHandler(true)
	seedTrigger(t, ts, "Alpha")
	seedTrigger(t, ts, "Beta")
	req := httptest.NewRequest("GET", "/api/v1/alert-triggers", nil)
	rec := httptest.NewRecorder()
	h.HandleListTriggers(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Alpha")
	assert.Contains(t, rec.Body.String(), "Beta")
}

// ---------------------------------------------------------------------------
// HandleGetTrigger
// ---------------------------------------------------------------------------

func TestHandleGetTrigger_Found(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "MyTrigger")
	req := httptest.NewRequest("GET", "/api/v1/alert-triggers/1", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleGetTrigger(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "MyTrigger")
}

func TestHandleGetTrigger_NotFound(t *testing.T) {
	h, _ := newTriggerHandler(true)
	req := httptest.NewRequest("GET", "/api/v1/alert-triggers/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	h.HandleGetTrigger(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "trigger_not_found")
}

// ---------------------------------------------------------------------------
// HandleCreateTrigger
// ---------------------------------------------------------------------------

func TestHandleCreateTrigger_Happy(t *testing.T) {
	h, _ := newTriggerHandler(true)
	body := `{"name":"Critical","filter_severities":"critical","filter_sources":"container","enabled":true,"channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "Critical")
}

func TestHandleCreateTrigger_EmptyName(t *testing.T) {
	h, _ := newTriggerHandler(true)
	body := `{"name":"","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "validation_failed")
}

func TestHandleCreateTrigger_EmptyChannelIDs(t *testing.T) {
	h, _ := newTriggerHandler(true)
	body := `{"name":"My trigger","channel_ids":[]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "validation_failed")
}

func TestHandleCreateTrigger_ChannelNotFound(t *testing.T) {
	h, _ := newTriggerHandler(false) // channel store returns nil
	body := `{"name":"My trigger","channel_ids":["99"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "validation_failed")
}

func TestHandleCreateTrigger_NameConflict(t *testing.T) {
	h, ts := newTriggerHandler(true)
	ts.insertErr = fmt.Errorf("insert trigger: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique})
	body := `{"name":"Existing","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "name_conflict")
}

func TestHandleCreateTrigger_NotifyOnResolve_DefaultsTrue(t *testing.T) {
	h, ts := newTriggerHandler(true)
	body := `{"name":"NoField","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
	got := ts.triggers["1"]
	require.NotNil(t, got)
	assert.True(t, got.NotifyOnResolve)
}

func TestHandleCreateTrigger_NotifyOnResolve_False(t *testing.T) {
	h, ts := newTriggerHandler(true)
	body := `{"name":"FiresOnly","notify_on_resolve":false,"channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
	got := ts.triggers["1"]
	require.NotNil(t, got)
	assert.False(t, got.NotifyOnResolve)
}

func TestHandleCreateTrigger_ProFilterScopes_CommunityBlocked(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	h, _ := newTriggerHandler(true)
	body := `{"name":"Scoped","filter_scopes":"container:42","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assertAdvancedFiltersRefusal(t, rec)
}

func TestHandleUpdateTrigger_ProFilterScopes_CommunityBlocked(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Original")
	body := `{"name":"Scoped","filter_scopes":"container:42","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assertAdvancedFiltersRefusal(t, rec)
	assert.Equal(t, "Original", ts.triggers[id].Name, "a refused update leaves the trigger alone")
}

func assertAdvancedFiltersRefusal(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusForbidden, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "EDITION_REQUIRED", body.Error.Code)
	assert.Equal(t, string(extension.CapAlertAdvancedFilters), body.Error.Feature)
	assert.Equal(t, string(extension.MinEdition(extension.CapAlertAdvancedFilters)), body.Error.RequiredEdition)
}

func TestHandleCreateTrigger_HasNoTagFilter(t *testing.T) {
	h, _ := newTriggerHandler(true)
	body := `{"name":"Plain","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.NotContains(t, got, "filter_tags")
}

func TestHandleCreateTrigger_ProFilterAllowed_Pro(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	defer func() { extension.CurrentEdition = original }()

	h, _ := newTriggerHandler(true)
	body := `{"name":"EntScoped","filter_scopes":"container:7","channel_ids":["1"]}`
	req := httptest.NewRequest("POST", "/api/v1/alert-triggers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateTrigger(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

// ---------------------------------------------------------------------------
// HandleUpdateTrigger
// ---------------------------------------------------------------------------

func TestHandleUpdateTrigger_Happy(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Original")
	body := `{"name":"Updated","filter_severities":"warning","enabled":true,"channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Updated")
}

func TestHandleUpdateTrigger_NotifyOnResolve_False(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Original")
	body := `{"name":"Updated","notify_on_resolve":false,"channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := ts.triggers[id]
	require.NotNil(t, got)
	assert.False(t, got.NotifyOnResolve)
}

func TestHandleUpdateTrigger_NotifyOnResolve_OmittedKeepsExisting(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Original")
	ts.triggers[id].NotifyOnResolve = false

	body := `{"name":"Updated","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := ts.triggers[id]
	require.NotNil(t, got)
	assert.False(t, got.NotifyOnResolve, "omitted field must keep the existing value")
}

func TestHandleUpdateTrigger_Enabled_OmittedKeepsExisting(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Original")
	ts.triggers[id].Enabled = false

	body := `{"name":"Renamed","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, ts.triggers[id].Enabled, "an update that does not mention enabled must not switch the trigger on")
}

func TestHandleUpdateTrigger_UnchangedScopes_AllowedAfterDowngrade(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Scoped")
	ts.triggers[id].FilterScopes = "container:42"

	for _, enabled := range []bool{false, true} {
		body := fmt.Sprintf(`{"name":"Scoped","filter_scopes":"container:42","enabled":%t,"channel_ids":["1"]}`, enabled)
		req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.HandleUpdateTrigger(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, enabled, ts.triggers[id].Enabled)
	}
}

func TestHandleUpdateTrigger_ChangedScopes_RefusedAfterDowngrade(t *testing.T) {
	original := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Community }
	defer func() { extension.CurrentEdition = original }()

	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "Scoped")
	ts.triggers[id].FilterScopes = "container:42"

	body := `{"name":"Scoped","filter_scopes":"container:43","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assertAdvancedFiltersRefusal(t, rec)
	assert.Equal(t, "container:42", ts.triggers[id].FilterScopes)
}

func TestHandleUpdateTrigger_NotFound(t *testing.T) {
	h, _ := newTriggerHandler(true)
	body := `{"name":"X","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "trigger_not_found")
}

func TestHandleUpdateTrigger_ValidationFailed(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "ForUpdate")
	body := `{"name":"","channel_ids":["1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/alert-triggers/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleUpdateTrigger(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "validation_failed")
}

// ---------------------------------------------------------------------------
// HandleDeleteTrigger
// ---------------------------------------------------------------------------

func TestHandleDeleteTrigger_Happy(t *testing.T) {
	h, ts := newTriggerHandler(true)
	id := seedTrigger(t, ts, "ToDel")
	req := httptest.NewRequest("DELETE", "/api/v1/alert-triggers/1", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.HandleDeleteTrigger(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleDeleteTrigger_NotFound(t *testing.T) {
	h, _ := newTriggerHandler(true)
	req := httptest.NewRequest("DELETE", "/api/v1/alert-triggers/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()
	h.HandleDeleteTrigger(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "trigger_not_found")
}
