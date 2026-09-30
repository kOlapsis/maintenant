// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/kolapsis/maintenant/internal/api/v1"
)

const wiringWebhookBody = `{"name":"hook","url":"https://8.8.8.8/hook","event_types":["*"]}`

var crossSite = map[string]string{
	"Sec-Fetch-Site": "cross-site",
	"Origin":         "https://evil.example",
	"Content-Type":   "text/plain",
}

func newTestApp(t *testing.T, edit func(*Config)) (*App, *syncBuffer) {
	t.Helper()
	cfg, logs, logger := storageEnv(t)
	if edit != nil {
		edit(&cfg)
	}
	a, err := New(cfg, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	a.db.StartWriter(ctx)
	t.Cleanup(func() {
		cancel()
		_ = a.db.Close()
	})
	return a, logs
}

func serve(a *App, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.srv.Handler.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body v1.ErrorResponse
	if json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		return ""
	}
	return body.Error.Code
}

func TestHTTPServer_CrossOriginGuardCoversTheAPIOnly(t *testing.T) {
	a, _ := newTestApp(t, nil)

	rec := serve(a, http.MethodPost, "/api/v1/webhooks", wiringWebhookBody, crossSite)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "CROSS_ORIGIN_REFUSED", errorCode(t, rec))

	rec = serve(a, http.MethodPost, "/api/v1/webhooks", wiringWebhookBody,
		map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"})
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	for _, target := range []string{"/ping/0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e11", "/status/subscribe"} {
		rec = serve(a, http.MethodPost, target, `{}`, crossSite)
		assert.NotEqual(t, "CROSS_ORIGIN_REFUSED", errorCode(t, rec), "%s is called cross-origin by design", target)
	}
}

func TestHTTPServer_DemoDriverStillWrites(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.DemoMode = true
		c.DemoToken = "demo-driver-token"
	})

	for name, headers := range map[string]map[string]string{
		"seeding job":      {v1.DemoTokenHeader: "demo-driver-token"},
		"same-origin page": {v1.DemoTokenHeader: "demo-driver-token", "Sec-Fetch-Site": "same-origin"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(a, http.MethodPost, "/api/v1/webhooks", wiringWebhookBody, headers)
			assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		})
	}

	rec := serve(a, http.MethodPost, "/api/v1/webhooks", wiringWebhookBody, nil)
	assert.Equal(t, "DEMO_MODE", errorCode(t, rec))
}

const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

func TestHTTPServer_MCPStreamIsNotBuffered(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.MCP.Enabled = true
		c.MCP.AllowUnauthenticated = true
	})

	rec := serve(a, http.MethodPost, "/mcp", initializeRequest, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json, text/event-stream",
	})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
}

func TestHTTPServer_WarnsAboutAShortMCPClientSecret(t *testing.T) {
	for name, tc := range map[string]struct {
		secret string
		warned bool
	}{
		"short":        {"hunter2", true},
		"31 chars":     {strings.Repeat("a", 31), true},
		"32 chars":     {strings.Repeat("a", 32), false},
		"openssl -hex": {strings.Repeat("0f", 32), false},
	} {
		t.Run(name, func(t *testing.T) {
			a, logs := newTestApp(t, func(c *Config) {
				c.MCP.Enabled = true
				c.MCP.ClientID = "claude"
				c.MCP.ClientSecret = tc.secret
				c.BaseURL = "https://maintenant.example.com"
			})
			require.NotNil(t, a.srv, "a weak secret warns, it never stops the server")

			if tc.warned {
				assert.Contains(t, logs.String(), "level=WARN")
				assert.Contains(t, logs.String(), "MAINTENANT_MCP_CLIENT_SECRET is shorter than 32 characters")
			} else {
				assert.NotContains(t, logs.String(), "MAINTENANT_MCP_CLIENT_SECRET is shorter")
			}
			assert.NotContains(t, logs.String(), tc.secret, "the secret never reaches the logs")
		})
	}
}

func TestHTTPServer_OAuthEndpointsStayOpenCrossOrigin(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.MCP.Enabled = true
		c.MCP.ClientID = "claude"
		c.MCP.ClientSecret = strings.Repeat("0f", 32)
		c.BaseURL = "https://maintenant.example.com"
	})

	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"claude"}}.Encode()
	rec := serve(a, http.MethodPost, "/oauth/token", form, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "https://claude.ai",
		"Content-Type":   "application/x-www-form-urlencoded",
	})

	assert.NotEqual(t, http.StatusForbidden, rec.Code, rec.Body.String())
}
