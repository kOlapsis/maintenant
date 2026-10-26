// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 043-public-surfaces-hardening, US4 (C4): FR-030 to FR-033.

// US4 scenario 1, FR-031, SC-005: a foreign Host gets 421 on the API and on every page of the web app.
func TestHTTPServer_RefusesAForeignHostOnTheAPIAndTheDashboard(t *testing.T) {
	a, _ := newTestApp(t, nil)

	for _, target := range []string{"/api/v1/health", "/api/v1/containers", "/api/v1/heartbeats", "/", "/dashboard", "/containers"} {
		rec := hit(a, http.MethodGet, target, host("evil.example"))
		assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, "%s: %s", target, rec.Body.String())
		assert.Equal(t, "misdirected_request", errorCode(t, rec), target)
		assert.NotContains(t, rec.Body.String(), `"status":"ok"`, "%s: nothing is served", target)
		assert.NotContains(t, rec.Body.String(), "<html", "%s: nothing is served", target)
	}

	rec := hit(a, http.MethodPost, "/api/v1/webhooks", func(r *http.Request) {
		r.Host = "evil.example"
		r.Body = http.NoBody
		r.Header.Set("Content-Type", "application/json")
	})
	assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, rec.Body.String())
}

// US4 scenario 2, FR-030, SC-007: localhost, the loopback addresses, the listen address and the configured
// URLs are served, whatever the port, the case or the IPv6 brackets.
func TestHTTPServer_ServesTheExpectedHosts(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.Addr = "192.0.2.10:0"
		c.BaseURL = "https://now.example.com"
		c.StatusURL = "https://now.example.com/status"
	})
	hb := newHeartbeat(t, a, "job")

	for _, h := range []string{"localhost", "localhost:8080", "LOCALHOST", "127.0.0.1", "127.0.0.1:8080", "[::1]", "[::1]:8080", "192.0.2.10:8080", "now.example.com", "now.example.com:443", "NOW.Example.com"} {
		t.Run(h, func(t *testing.T) {
			rec := hit(a, http.MethodGet, "/api/v1/health", host(h))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"status":"ok"`)

			rec = hit(a, http.MethodGet, "/containers", host(h))
			assert.Equal(t, http.StatusOK, rec.Code, "the web app is served: %s", rec.Body.String())

			rec = hit(a, http.MethodGet, "/status/api", host(h))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			rec = hit(a, http.MethodGet, "/ping/"+hb.PingToken, host(h))
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

// US4 scenario 3, FR-030: an explicit list replaces the derived hosts; localhost and the loopback stay.
func TestHTTPServer_ExplicitAllowedHostsReplaceTheDerivedOnes(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.BaseURL = "https://app.example.com"
		c.StatusURL = "https://status.example.com"
		c.AllowedHosts = "now.example.com, monitor.lan ,,MONITOR2.LAN:8443, "
	})

	for _, h := range []string{"now.example.com", "monitor.lan", "MONITOR.LAN:8080", "monitor2.lan", "localhost", "localhost:8080", "127.0.0.1:8080", "[::1]:8080"} {
		rec := hit(a, http.MethodGet, "/api/v1/health", host(h))
		assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", h, rec.Body.String())
	}
	for _, h := range []string{"evil.example", "app.example.com", "monitor.lan.evil.example"} {
		rec := hit(a, http.MethodGet, "/api/v1/health", host(h))
		assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, "%s is not in the explicit list: %s", h, rec.Body.String())
	}

	blank, _ := newTestApp(t, func(c *Config) {
		c.BaseURL = "https://app.example.com"
		c.AllowedHosts = " , ,"
	})
	rec := hit(blank, http.MethodGet, "/api/v1/health", host("app.example.com"))
	assert.Equal(t, http.StatusOK, rec.Code, "a list without a valid entry falls back to the derived hosts: %s", rec.Body.String())
	rec = hit(blank, http.MethodGet, "/api/v1/health", host("evil.example"))
	assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, rec.Body.String())
}

// US4 scenario 4, FR-032: the public routes, the assets, the installable app files, /mcp, /oauth/ and
// /.well-known/ answer whatever the Host.
func TestHTTPServer_HostIsNotCheckedOnPublicRoutes(t *testing.T) {
	a, _ := newTestApp(t, func(c *Config) {
		c.BaseURL = "https://now.example.com"
		c.AllowedHosts = "now.example.com"
		c.MCP.Enabled = true
		c.MCP.ClientID = "claude"
		c.MCP.ClientSecret = strings.Repeat("0f", 32)
	})
	a.statusHandler.SetIndexHTML(testIndexHTML)
	hb := newHeartbeat(t, a, "job")
	evil := host("evil.example")

	rec := hit(a, http.MethodGet, "/ping/"+hb.PingToken, evil)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = hit(a, http.MethodPost, "/ping/"+hb.PingToken+"/start", evil)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = hit(a, http.MethodGet, "/ping/"+uid.New(), evil)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "HEARTBEAT_NOT_FOUND", errorCode(t, rec))

	for _, target := range []string{"/status", "/status/", "/status/api", "/status/feed.atom"} {
		rec := hit(a, http.MethodGet, target, evil)
		assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", target, rec.Body.String())
	}

	for _, target := range []string{
		"/assets/index.js", "/manifest.webmanifest", "/sw.js", "/registerSW.js", "/workbox-8b1f5376.js",
		"/favicon.ico", "/favicon.svg", "/icon-192.png", "/apple-touch-icon.png", "/logo.svg",
		"/mcp", "/mcp/", "/oauth/authorize", "/oauth/token", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource",
	} {
		foreign := hit(a, http.MethodGet, target, evil)
		expected := hit(a, http.MethodGet, target, host("now.example.com"))
		assert.NotEqual(t, http.StatusMisdirectedRequest, foreign.Code, target)
		assert.Equal(t, expected.Code, foreign.Code, "%s answers a foreign Host like the expected one", target)
	}

	rec = hit(a, http.MethodGet, "/api/v1/health", evil)
	assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, "the guard is on for the API in this configuration")
}

// Edge case: a request without a Host header (HTTP/1.0) is refused on the API and the web app, not on the public routes.
func TestHTTPServer_RequestWithoutHostIsRefusedOnTheAPI(t *testing.T) {
	a, _ := newTestApp(t, nil)

	for _, target := range []string{"/api/v1/health", "/dashboard"} {
		rec := hit(a, http.MethodGet, target, host(""))
		assert.Equal(t, http.StatusMisdirectedRequest, rec.Code, "%s: %s", target, rec.Body.String())
		assert.Equal(t, "misdirected_request", errorCode(t, rec), target)
	}
	rec := hit(a, http.MethodGet, "/status/api", host(""))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// US4 scenario 5, FR-033: listening beyond the loopback without MAINTENANT_ALLOWED_HOSTS warns at startup,
// naming the variable and the default list.
func TestStart_WarnsWhenListeningBeyondLoopbackWithoutAllowedHosts(t *testing.T) {
	cases := []struct {
		name, ip, allowed string
		warned            bool
	}{
		{"every interface without the variable", "0.0.0.0", "", true},
		{"loopback without the variable", "127.0.0.1", "", false},
		{"every interface with the variable", "0.0.0.0", "now.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			_, port, err := net.SplitHostPort(probe.Addr().String())
			require.NoError(t, err)
			require.NoError(t, probe.Close())

			cfg, logs, logger := storageEnv(t)
			cfg.Addr = net.JoinHostPort(tc.ip, port)
			cfg.AllowedHosts = tc.allowed
			a, err := New(cfg, logger)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- a.Start(ctx) }()
			require.Eventually(t, func() bool {
				resp, err := http.Get("http://127.0.0.1:" + port + "/api/v1/health")
				if err != nil {
					return false
				}
				_ = resp.Body.Close()
				return true
			}, 5*time.Second, 50*time.Millisecond, "the HTTP server must come up")
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("Start did not return")
			}

			lines := warnLines(logs, "MAINTENANT_ALLOWED_HOSTS")
			if !tc.warned {
				assert.Empty(t, lines, "no warning expected")
				return
			}
			require.Len(t, lines, 1, "one warning naming the variable, got %q", logs.String())
			assert.Contains(t, lines[0], "localhost", "the warning lists the hosts accepted by default")
		})
	}
}
