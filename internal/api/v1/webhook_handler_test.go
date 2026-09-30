// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubWebhookStore struct {
	created   *webhook.WebhookSubscription
	sub       *webhook.WebhookSubscription
	delivered []bool
}

func (s *stubWebhookStore) List(context.Context) ([]*webhook.WebhookSubscription, error) {
	return nil, nil
}
func (s *stubWebhookStore) GetByID(context.Context, string) (*webhook.WebhookSubscription, error) {
	return s.sub, nil
}
func (s *stubWebhookStore) Create(_ context.Context, sub *webhook.WebhookSubscription) error {
	s.created = sub
	return nil
}
func (s *stubWebhookStore) Delete(context.Context, string) error { return nil }
func (s *stubWebhookStore) RecordDelivery(_ context.Context, _ string, delivered bool) error {
	s.delivered = append(s.delivered, delivered)
	return nil
}
func (s *stubWebhookStore) ListActive(context.Context) ([]*webhook.WebhookSubscription, error) {
	return nil, nil
}

// A webhook subscription must never be created for an internal/private URL
// (SSRF). The only accepted destinations are public HTTPS endpoints, unless
// AllowPrivateWebhooks is explicitly enabled for local dev.
func TestHandleCreateWebhook_SSRF(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name         string
		allowPrivate bool
		url          string
		wantStatus   int
	}{
		{"http scheme rejected", false, "http://example.com/hook", http.StatusBadRequest},
		{"loopback rejected", false, "https://127.0.0.1/hook", http.StatusBadRequest},
		{"cloud imds rejected", false, "https://169.254.169.254/latest/meta-data/", http.StatusBadRequest},
		{"private ip rejected", false, "https://10.0.0.5/hook", http.StatusBadRequest},
		{"public https allowed", false, "https://8.8.8.8/hook", http.StatusCreated},
		{"dev mode allows private", true, "https://127.0.0.1/hook", http.StatusCreated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubWebhookStore{}
			h := NewWebhookHandler(store, nil, logger, tc.allowPrivate)

			body := `{"name":"hook","url":"` + tc.url + `","event_types":["*"]}`
			req := httptest.NewRequest("POST", "/api/v1/webhooks", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.HandleCreateWebhook(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			if tc.wantStatus == http.StatusBadRequest {
				assert.Nil(t, store.created, "no subscription must be persisted for a rejected URL")
			}
		})
	}
}

func TestHandleTestWebhook_SignsLikeARealDelivery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const secret = "s3cr3t"

	var gotBody []byte
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader = r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	store := &stubWebhookStore{sub: &webhook.WebhookSubscription{
		ID: "w1", Name: "hook", URL: srv.URL, Secret: secret, EventTypes: []string{"*"}, IsActive: true,
	}}
	dispatcher := webhook.NewDispatcher(store, alert.NewNotifier(nil, logger, true), logger)
	h := NewWebhookHandler(store, dispatcher, logger, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/w1/test", nil)
	req.SetPathValue("id", "w1")
	rec := httptest.NewRecorder()
	h.HandleTestWebhook(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "delivered", result["status"])
	assert.EqualValues(t, http.StatusAccepted, result["http_status"])

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), gotHeader.Get("X-maintenant-Signature"))
	assert.Equal(t, "test", gotHeader.Get("X-maintenant-Event"))
	assert.NotEmpty(t, gotHeader.Get("X-maintenant-Delivery"))
	assert.Equal(t, []bool{true}, store.delivered, "a test is recorded like any delivery")
}
