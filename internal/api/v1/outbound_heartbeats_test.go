// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/outbound"
)

type memOutboundStore struct {
	mu    sync.Mutex
	items map[string]outbound.OutboundHeartbeat
}

func (m *memOutboundStore) List(context.Context) ([]*outbound.OutboundHeartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*outbound.OutboundHeartbeat
	for _, o := range m.items {
		cp := o
		out = append(out, &cp)
	}
	return out, nil
}

func (m *memOutboundStore) Get(_ context.Context, id string) (*outbound.OutboundHeartbeat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.items[id]
	if !ok {
		return nil, nil
	}
	return &o, nil
}

func (m *memOutboundStore) Create(_ context.Context, o *outbound.OutboundHeartbeat) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[o.ID] = *o
	return nil
}

func (m *memOutboundStore) Update(_ context.Context, o *outbound.OutboundHeartbeat) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[o.ID]; !ok {
		return false, nil
	}
	m.items[o.ID] = *o
	return true, nil
}

func (m *memOutboundStore) Delete(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.items[id]
	delete(m.items, id)
	return ok, nil
}

func (m *memOutboundStore) RecordSend(_ context.Context, id string, r outbound.SendResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := m.items[id]
	at := r.SentAt
	o.LastSentAt, o.LastStatusCode, o.LastError = &at, r.StatusCode, r.Error
	m.items[id] = o
	return nil
}

func newOutboundTestMux() *http.ServeMux {
	oh := NewOutboundHeartbeatHandler(outbound.NewService(outbound.Deps{
		Store:       &memOutboundStore{items: map[string]outbound.OutboundHeartbeat{}},
		ValidateURL: func(context.Context, string) error { return nil },
	}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/outbound-heartbeats", oh.HandleList)
	mux.HandleFunc("POST /api/v1/outbound-heartbeats", oh.HandleCreate)
	mux.HandleFunc("PUT /api/v1/outbound-heartbeats/{id}", oh.HandleUpdate)
	mux.HandleFunc("DELETE /api/v1/outbound-heartbeats/{id}", oh.HandleDelete)
	mux.HandleFunc("POST /api/v1/outbound-heartbeats/{id}/send", oh.HandleSend)
	return mux
}

func doOutbound(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestOutboundHeartbeatHandler_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{"malformed json", `{`, "INVALID_JSON"},
		{"missing name", `{"url":"https://h/ping/x","interval_seconds":60}`, "INVALID_INPUT"},
		{"bad scheme", `{"name":"a","url":"ftp://h/x","interval_seconds":60}`, "INVALID_INPUT"},
		{"plain http", `{"name":"a","url":"http://h/ping/x","interval_seconds":60}`, "INVALID_INPUT"},
		{"relative url", `{"name":"a","url":"/ping/x","interval_seconds":60}`, "INVALID_INPUT"},
		{"interval below 30", `{"name":"a","url":"https://h","interval_seconds":10}`, "INVALID_INPUT"},
		{"interval above 86400", `{"name":"a","url":"https://h","interval_seconds":90000}`, "INVALID_INPUT"},
	}
	mux := newOutboundTestMux()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doOutbound(t, mux, http.MethodPost, "/api/v1/outbound-heartbeats", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			var resp ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.code, resp.Error.Code)
		})
	}
}

func TestOutboundHeartbeatHandler_UnknownID(t *testing.T) {
	mux := newOutboundTestMux()
	valid := `{"name":"a","url":"https://h","interval_seconds":60}`
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/outbound-heartbeats/nope", valid},
		{http.MethodDelete, "/api/v1/outbound-heartbeats/nope", ""},
		{http.MethodPost, "/api/v1/outbound-heartbeats/nope/send", ""},
	} {
		rec := doOutbound(t, mux, tc.method, tc.path, tc.body)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s %s", tc.method, tc.path)
	}
}

func TestOutboundHeartbeatHandler_CreateListDelete(t *testing.T) {
	mux := newOutboundTestMux()
	rec := doOutbound(t, mux, http.MethodPost, "/api/v1/outbound-heartbeats",
		`{"name":"upstream","url":"https://mnt.example/ping/abc","interval_seconds":60,"enabled":false}`)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	for _, key := range []string{"id", "name", "url", "interval_seconds", "enabled",
		"last_sent_at", "last_status_code", "last_error", "created_at", "updated_at"} {
		assert.Contains(t, created, key)
	}
	assert.Equal(t, false, created["enabled"])
	assert.Nil(t, created["last_sent_at"])
	id := created["id"].(string)

	rec = doOutbound(t, mux, http.MethodGet, "/api/v1/outbound-heartbeats", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		OutboundHeartbeats []map[string]any `json:"outbound_heartbeats"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.OutboundHeartbeats, 1)

	rec = doOutbound(t, mux, http.MethodDelete, "/api/v1/outbound-heartbeats/"+id, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doOutbound(t, mux, http.MethodGet, "/api/v1/outbound-heartbeats", "")
	assert.JSONEq(t, `{"outbound_heartbeats":[]}`, rec.Body.String())
}
