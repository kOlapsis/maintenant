// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	commesc "github.com/kolapsis/maintenant/internal/commercial/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func realEscalationMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	chID, err := channels.InsertChannel(context.Background(), &alert.NotificationChannel{
		Name: "ops-webhook", Type: "webhook", URL: "https://example.com/hook", Enabled: true,
	})
	require.NoError(t, err)
	svc := commesc.NewService(store.NewEscalationStore(db), channels,
		func() extension.Edition { return extension.Pro }, noopSuppressor{}, logger)
	h := NewEscalationHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/escalation-policies", h.HandleCreatePolicy)
	mux.HandleFunc("PUT /api/v1/escalation-policies/{id}", h.HandleUpdatePolicy)
	return mux, chID
}

func policyRequestJSON(t *testing.T, severities []string, channelIDs ...string) []byte {
	t.Helper()
	body, err := json.Marshal(escalation.PolicyRequest{
		Name:    "validated",
		Filters: escalation.Filters{Severities: severities, Scopes: []escalation.Scope{}},
		Levels:  []escalation.LevelReq{{DelaySeconds: 300, ChannelIDs: channelIDs}},
	})
	require.NoError(t, err)
	return body
}

func doEscalation(mux http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestEscalationPolicy_RealStore_RejectsInvalid(t *testing.T) {
	mux, chID := realEscalationMux(t)

	cases := []struct {
		name string
		body []byte
	}{
		{"unknown channel", policyRequestJSON(t, []string{"critical"}, "00000000-0000-0000-0000-00000000dead")},
		{"channel name instead of id", policyRequestJSON(t, []string{"critical"}, "ops-webhook")},
		{"empty channel id", policyRequestJSON(t, []string{"critical"}, chID, "")},
		{"repeated channel", policyRequestJSON(t, []string{"critical"}, chID, chID)},
		{"capitalised severity", policyRequestJSON(t, []string{"Critical"}, chID)},
		{"unknown severity", policyRequestJSON(t, []string{"critical", "fatal"}, chID)},
	}
	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			rec := doEscalation(mux, http.MethodPost, "/api/v1/escalation-policies", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "validation_failed")
		})
	}

	created := doEscalation(mux, http.MethodPost, "/api/v1/escalation-policies", policyRequestJSON(t, []string{"critical", "warning", "info"}, chID))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var p escalation.Policy
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &p))

	for _, tc := range cases {
		t.Run("update/"+tc.name, func(t *testing.T) {
			rec := doEscalation(mux, http.MethodPut, "/api/v1/escalation-policies/"+p.ID, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
	ok := doEscalation(mux, http.MethodPut, "/api/v1/escalation-policies/"+p.ID, policyRequestJSON(t, nil, chID))
	assert.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
}

type failingChannelStore struct{ alert.ChannelStore }

func (failingChannelStore) GetChannel(context.Context, string) (*alert.NotificationChannel, error) {
	return nil, errors.New("database unavailable")
}

func TestEscalationPolicy_ChannelStoreFailureIsNotValidation(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	svc := commesc.NewService(store.NewEscalationStore(db), failingChannelStore{store.NewChannelStore(db)},
		func() extension.Edition { return extension.Pro }, noopSuppressor{}, logger)
	h := NewEscalationHandler(svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/escalation-policies", bytes.NewReader(policyRequestJSON(t, []string{"critical"}, "1")))
	h.HandleCreatePolicy(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}
