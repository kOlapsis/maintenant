// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const webhookBody = `{"name":"hook","url":"https://8.8.8.8/hook","event_types":["*"]}`

func webhookRouter(t *testing.T, corsOrigins string, logs io.Writer) (*Router, *stubWebhookStore) {
	t.Helper()
	if logs == nil {
		logs = io.Discard
	}
	store := &stubWebhookStore{}
	r := NewRouter(HandlerDeps{
		Logger:       slog.New(slog.NewTextHandler(logs, nil)),
		WebhookStore: store,
		CORSOrigins:  corsOrigins,
	})
	return r, store
}

func sendWebhook(r *Router, method string, headers map[string]string) *httptest.ResponseRecorder {
	var body io.Reader
	if method != http.MethodGet {
		body = strings.NewReader(webhookBody)
	}
	req := httptest.NewRequest(method, "http://maintenant.example.com/api/v1/webhooks", body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	return rec
}

func TestCrossOrigin_RefusesCrossSiteWrite(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"modern browser": {
			"Sec-Fetch-Site": "cross-site",
			"Origin":         "https://evil.example",
			"Content-Type":   "text/plain",
		},
		"same site, other origin": {
			"Sec-Fetch-Site": "same-site",
			"Origin":         "https://blog.example.com",
			"Content-Type":   "text/plain",
		},
		"browser without Sec-Fetch-Site": {
			"Origin":       "https://evil.example",
			"Content-Type": "text/plain",
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, store := webhookRouter(t, "", nil)

			rec := sendWebhook(r, http.MethodPost, headers)

			require.Equal(t, http.StatusForbidden, rec.Code)
			assert.Nil(t, store.created, "a refused request must never reach the handler")
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "CROSS_ORIGIN_REFUSED", body.Error.Code)
			assert.Contains(t, body.Error.Message, "MAINTENANT_CORS_ORIGINS")
		})
	}
}

func TestCrossOrigin_AllowsSameOriginAndNonBrowserWrites(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"same origin":                  {"Sec-Fetch-Site": "same-origin", "Origin": "http://maintenant.example.com"},
		"user initiated":               {"Sec-Fetch-Site": "none"},
		"old browser, Origin is Host":  {"Origin": "http://maintenant.example.com"},
		"script without Origin (curl)": {},
	} {
		t.Run(name, func(t *testing.T) {
			r, store := webhookRouter(t, "", nil)

			rec := sendWebhook(r, http.MethodPost, headers)

			assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			assert.NotNil(t, store.created)
		})
	}
}

func TestCrossOrigin_TrustsListedCORSOrigins(t *testing.T) {
	r, store := webhookRouter(t, "https://ops.example.org, https://grafana.example.org", nil)

	rec := sendWebhook(r, http.MethodPost, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://grafana.example.org",
		"Content-Type":   "application/json",
	})
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotNil(t, store.created)

	rec = sendWebhook(r, http.MethodPost, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://evil.example",
	})
	assert.Equal(t, http.StatusForbidden, rec.Code, "only the listed origins are trusted")
}

func TestCrossOrigin_WildcardCORSTrustsNoOrigin(t *testing.T) {
	r, store := webhookRouter(t, "*", nil)

	rec := sendWebhook(r, http.MethodPost, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://evil.example",
	})

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, store.created)
}

func TestCrossOrigin_NeverBlocksSafeMethods(t *testing.T) {
	r, _ := webhookRouter(t, "", nil)

	rec := sendWebhook(r, http.MethodGet, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://evil.example",
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCrossOrigin_LeavesPingRoutesOpen(t *testing.T) {
	var reached bool
	h := crossOriginGuard(http.NewCrossOriginProtection(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/ping/0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e11", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://ci.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, reached, "a heartbeat ping may come from anywhere")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCrossOrigin_WarnsAboutAnEntryThatIsNotAnOrigin(t *testing.T) {
	var logs bytes.Buffer
	r, _ := webhookRouter(t, "https://ops.example.org/, https://grafana.example.org", &logs)

	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "https://ops.example.org/")

	rec := sendWebhook(r, http.MethodPost, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://grafana.example.org",
	})
	assert.Equal(t, http.StatusCreated, rec.Code, "the valid entries are still trusted")
}

func TestCORS_AllowsPatch(t *testing.T) {
	handler := cors([]string{"https://ops.example.org"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/agents/1", nil)
	req.Header.Set("Origin", "https://ops.example.org")
	req.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "PATCH")
}
