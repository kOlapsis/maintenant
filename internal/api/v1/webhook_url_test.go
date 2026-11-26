// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/webhook"
)

type listingWebhookStore struct {
	stubWebhookStore
	subs []*webhook.WebhookSubscription
}

func (s *listingWebhookStore) List(context.Context) ([]*webhook.WebhookSubscription, error) {
	return s.subs, nil
}

type failingWebhookTester struct{ err error }

func (f failingWebhookTester) Test(context.Context, *webhook.WebhookSubscription) (int, error) {
	return 0, f.err
}

func TestWebhooks_AnswersCutTheURLAfterTheHost(t *testing.T) {
	const full = "https://hooks.example.com:8443/in/S3CR3T?token=t0k3n"
	stored := &webhook.WebhookSubscription{ID: "w1", Name: "ops", URL: full, EventTypes: []string{"*"}, IsActive: true}
	store := &listingWebhookStore{subs: []*webhook.WebhookSubscription{stored}}
	store.sub = stored
	h := NewWebhookHandler(store, failingWebhookTester{errors.New(`Post "` + full + `": EOF`)}, slog.New(slog.DiscardHandler), true)
	secrets := func(t *testing.T, body string) {
		t.Helper()
		assert.NotContains(t, body, "S3CR3T")
		assert.NotContains(t, body, "t0k3n")
		assert.Contains(t, body, "https://hooks.example.com:8443")
	}

	rec := httptest.NewRecorder()
	h.HandleListWebhooks(rec, httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	secrets(t, rec.Body.String())
	var listed struct {
		Webhooks []map[string]any `json:"webhooks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Webhooks, 1)
	assert.Equal(t, "https://hooks.example.com:8443", listed.Webhooks[0]["url"])
	assert.Equal(t, "ops", listed.Webhooks[0]["name"])
	assert.Equal(t, full, stored.URL, "the stored subscription keeps its URL")

	rec = httptest.NewRecorder()
	h.HandleCreateWebhook(rec, httptest.NewRequest(http.MethodPost, "/api/v1/webhooks",
		strings.NewReader(`{"name":"new","url":"`+full+`"}`)))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	secrets(t, rec.Body.String())
	assert.Equal(t, full, store.created.URL)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/w1/test", nil)
	req.SetPathValue("id", "w1")
	h.HandleTestWebhook(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	secrets(t, rec.Body.String())
}
