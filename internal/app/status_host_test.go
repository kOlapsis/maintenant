// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 043-public-surfaces-hardening, US3 (C3): FR-020, FR-021, SC-004.

const statusMetaTag = `<meta name="maintenant-status" content="true">`

func statusHostApp(t *testing.T, statusURL string) *App {
	t.Helper()
	a, _ := newTestApp(t, func(c *Config) {
		c.BaseURL = "https://app.example.com"
		c.StatusURL = statusURL
		c.MCP.Enabled = true
		c.MCP.AllowUnauthenticated = true
	})
	a.statusHandler.SetIndexHTML(testIndexHTML)
	return a
}

// streamStatus opens /status/events for a moment and returns the response head.
func streamStatus(a *App, h string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rec := hit(a, http.MethodGet, "/status/events", func(r *http.Request) {
		r.Host = h
		*r = *r.WithContext(ctx)
	})
	return rec.Code, rec.Header().Get("Content-Type")
}

// US3 scenarios 1 and 2, FR-020, FR-021, SC-004: on the status host, only the public page and its files answer.
func TestHTTPServer_StatusHostServesOnlyThePublicPage(t *testing.T) {
	a := statusHostApp(t, "https://status.example.com")
	hb := newHeartbeat(t, a, "job")

	for _, h := range []string{"status.example.com", "status.example.com:443", "STATUS.EXAMPLE.COM"} {
		t.Run(h, func(t *testing.T) {
			for _, target := range []string{"/api/v1/containers", "/api/v1/health", "/api/v1/status/components", "/api/v1/heartbeats", "/containers", "/dashboard", "/status-admin", "/mcp", "/mcp/", "/oauth/token", "/statuses"} {
				rec := hit(a, http.MethodGet, target, host(h))
				assert.Equal(t, http.StatusNotFound, rec.Code, "%s on the status host: %s", target, rec.Body.String())
				assert.NotContains(t, rec.Body.String(), `"status":"ok"`, target)
			}
			rec := hit(a, http.MethodPost, "/api/v1/heartbeats", host(h))
			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

			for _, target := range []string{"/", "/status", "/status/"} {
				rec := hit(a, http.MethodGet, target, host(h))
				assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", target, rec.Body.String())
				assert.Contains(t, rec.Body.String(), statusMetaTag, "%s serves the status page, so the web app detects the status context", target)
			}

			rec = hit(a, http.MethodGet, "/status/api", host(h))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"global_status"`)

			code, contentType := streamStatus(a, h)
			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "text/event-stream", contentType)

			rec = hit(a, http.MethodGet, "/ping/"+hb.PingToken, host(h))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			for _, target := range []string{
				"/assets/index.js", "/assets/missing.css", "/manifest.webmanifest", "/sw.js", "/registerSW.js", "/workbox-8b1f5376.js",
				"/favicon.ico", "/favicon.svg", "/icon-192.png", "/icon.svg", "/apple-touch-icon.png", "/logo.svg", "/screenshots/dashboard.png",
			} {
				onStatusHost := hit(a, http.MethodGet, target, host(h))
				onAppHost := hit(a, http.MethodGet, target, host("app.example.com"))
				assert.Equal(t, onAppHost.Code, onStatusHost.Code, "%s is served on the status host as on the app host", target)
			}
		})
	}

	rec := hit(a, http.MethodGet, "/", host("app.example.com"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), statusMetaTag, "/ on the app host is the dashboard, not the status page")
}

// US3 scenarios 3 and 4, edge case "status host equal to the base host": the filter only exists for a dedicated subdomain.
func TestHTTPServer_StatusHostFilterOnlyAppliesToADedicatedSubdomain(t *testing.T) {
	dedicated := statusHostApp(t, "https://status.example.com")
	for _, target := range []string{"/api/v1/containers", "/api/v1/health", "/containers"} {
		rec := hit(dedicated, http.MethodGet, target, host("app.example.com"))
		assert.Equal(t, http.StatusOK, rec.Code, "%s on the app host: %s", target, rec.Body.String())
	}
	rec := hit(dedicated, http.MethodGet, "/api/v1/containers", host("evil.example"))
	assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, "a foreign host is refused by the host list, not by the status filter")

	cases := map[string]struct{ base, status, host string }{
		"status page under the base host":             {"https://now.example.com", "https://now.example.com/status", "now.example.com"},
		"status page under the base host with a port": {"https://now.example.com:8443", "https://now.example.com:8443/status", "now.example.com:8443"},
		"no status url":            {"https://now.example.com", "", "now.example.com"},
		"no status url, localhost": {"", "", "localhost"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a, _ := newTestApp(t, func(c *Config) {
				c.BaseURL = tc.base
				c.StatusURL = tc.status
			})
			a.statusHandler.SetIndexHTML(testIndexHTML)
			for _, target := range []string{"/api/v1/containers", "/api/v1/health", "/containers"} {
				rec := hit(a, http.MethodGet, target, host(tc.host))
				assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", target, rec.Body.String())
			}
			rec := hit(a, http.MethodGet, "/", host(tc.host))
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotContains(t, rec.Body.String(), statusMetaTag, "/ stays the dashboard")
			rec = hit(a, http.MethodGet, "/status/api", host(tc.host))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

// Edge case: a status URL with a port is recognised with or without that port in the Host header.
func TestHTTPServer_StatusURLWithAPortIsRecognisedWithOrWithoutIt(t *testing.T) {
	a := statusHostApp(t, "https://status.example.com:8443")

	for _, h := range []string{"status.example.com:8443", "status.example.com", "status.example.com:443"} {
		rec := hit(a, http.MethodGet, "/api/v1/health", host(h))
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", h, rec.Body.String())
		rec = hit(a, http.MethodGet, "/status/api", host(h))
		assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", h, rec.Body.String())
		rec = hit(a, http.MethodGet, "/", host(h))
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", h, rec.Body.String())
		assert.Contains(t, rec.Body.String(), statusMetaTag, h)
	}
}
