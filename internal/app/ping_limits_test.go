// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 043-public-surfaces-hardening, US5 (C5): FR-040 to FR-043.

const floodAddr = "192.0.2.1:4000"

// US5 scenario 1, FR-041, SC-006: a flood of unknown tokens from one address does not starve a valid token from the same address.
func TestHTTPServer_UnknownPingsDoNotStarveAValidToken(t *testing.T) {
	a, _ := newTestApp(t, nil)
	hb := newHeartbeat(t, a, "nightly backup")

	for i := 0; i < 100; i++ {
		rec := hit(a, http.MethodGet, "/ping/"+uid.New(), from(floodAddr))
		assert.NotEqual(t, http.StatusOK, rec.Code, "ping %d: an unknown token is never accepted", i+1)
	}

	rec := hit(a, http.MethodGet, "/ping/"+hb.PingToken, from(floodAddr))
	require.Equal(t, http.StatusOK, rec.Code, "the valid ping right after the flood: %s", rec.Body.String())
	got, err := a.heartbeatSvc.GetHeartbeat(context.Background(), hb.ID)
	require.NoError(t, err)
	assert.Equal(t, heartbeat.StatusUp, got.Status)
}

// US5 scenario 2, FR-040 and the paused edge case: a known token has its own quota of 60 a minute,
// the excess answers 429 with Retry-After and is not recorded, the other tokens are untouched.
func TestHTTPServer_KnownTokenHasItsOwnQuotaOfSixtyPerMinute(t *testing.T) {
	a, _ := newTestApp(t, nil)

	for name, paused := range map[string]bool{"running": false, "paused": true} {
		t.Run(name, func(t *testing.T) {
			hb := newHeartbeat(t, a, "nightly backup "+name)
			other := newHeartbeat(t, a, "hourly sync "+name)
			if paused {
				_, err := a.heartbeatSvc.PauseHeartbeat(context.Background(), hb.ID)
				require.NoError(t, err)
			}

			first := hit(a, http.MethodGet, "/ping/"+hb.PingToken, from(floodAddr))
			require.NotEqual(t, http.StatusTooManyRequests, first.Code, first.Body.String())
			accepted, refused := 1, 0
			for i := 2; i <= 70; i++ {
				rec := hit(a, http.MethodGet, "/ping/"+hb.PingToken, from(floodAddr))
				if i <= 60 {
					assert.Equal(t, first.Code, rec.Code, "ping %d is within the quota: %s", i, rec.Body.String())
				}
				switch rec.Code {
				case http.StatusTooManyRequests:
					refused++
					assert.NotEmpty(t, rec.Header().Get("Retry-After"), "ping %d", i)
					assert.Equal(t, "rate_limited", errorCode(t, rec), "ping %d", i)
				case first.Code:
					accepted++
				default:
					t.Fatalf("ping %d: got %d %s", i, rec.Code, rec.Body.String())
				}
			}
			assert.GreaterOrEqual(t, refused, 1, "70 pings in a few seconds exceed 60 a minute")

			pings, _, err := a.heartbeatSvc.ListPings(context.Background(), hb.ID, heartbeat.ListPingsOpts{Limit: 100})
			require.NoError(t, err)
			assert.Len(t, pings, accepted, "a refused ping is not recorded")

			rec := hit(a, http.MethodGet, "/ping/"+other.PingToken, from(floodAddr))
			assert.Equal(t, http.StatusOK, rec.Code, "another token from the same address keeps its quota: %s", rec.Body.String())
			rec = hit(a, http.MethodGet, "/status/api", from(floodAddr))
			assert.Equal(t, http.StatusOK, rec.Code, "the status page keeps its own quota: %s", rec.Body.String())
		})
	}
}

// US5 scenario 3, FR-041, FR-042: unknown tokens are limited per address in a bucket of their own, apart from /status/.
func TestHTTPServer_UnknownPingsAreLimitedPerAddressApartFromTheStatusPage(t *testing.T) {
	a, _ := newTestApp(t, nil)

	refused := 0
	for i := 1; i <= 40; i++ {
		rec := hit(a, http.MethodGet, "/ping/"+uid.New(), from(floodAddr))
		switch rec.Code {
		case http.StatusNotFound:
			assert.Equal(t, "HEARTBEAT_NOT_FOUND", errorCode(t, rec), "ping %d", i)
		case http.StatusTooManyRequests:
			refused++
			assert.NotEmpty(t, rec.Header().Get("Retry-After"), "ping %d", i)
			assert.Equal(t, "rate_limited", errorCode(t, rec), "ping %d", i)
		default:
			t.Fatalf("ping %d: got %d %s", i, rec.Code, rec.Body.String())
		}
	}
	assert.GreaterOrEqual(t, refused, 1, "40 unknown pings in a row exceed 10 a second with a burst of 20")

	for i := 1; i <= 20; i++ {
		rec := hit(a, http.MethodGet, "/status/api", from(floodAddr))
		assert.Equal(t, http.StatusOK, rec.Code, "status request %d from the flooding address: %s", i, rec.Body.String())
	}

	rec := hit(a, http.MethodGet, "/ping/"+uid.New(), from("198.51.100.9:4000"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another address has its own bucket: %s", rec.Body.String())
}

// US5 scenario 4, FR-043: forwarded headers without a trusted proxy warn once per process, naming the variable.
func TestHTTPServer_WarnsOnceAboutForwardedHeadersWithoutTrustedProxies(t *testing.T) {
	a, logs := newTestApp(t, nil)
	hb := newHeartbeat(t, a, "job")

	for _, h := range []map[string]string{
		{"X-Forwarded-For": "198.51.100.7"},
		{"X-Forwarded-For": "198.51.100.8"},
		{"X-Real-IP": "198.51.100.9"},
	} {
		for _, target := range []string{"/status/api", "/ping/" + hb.PingToken, "/api/v1/health"} {
			rec := hit(a, http.MethodGet, target, func(r *http.Request) {
				for k, v := range h {
					r.Header.Set(k, v)
				}
			})
			assert.NotEqual(t, http.StatusTooManyRequests, rec.Code, target)
		}
	}
	lines := warnLines(logs, "MAINTENANT_TRUSTED_PROXIES")
	require.Len(t, lines, 1, "exactly one warning per process, got %q", logs.String())

	pings, _, err := a.heartbeatSvc.ListPings(context.Background(), hb.ID, heartbeat.ListPingsOpts{Limit: 10})
	require.NoError(t, err)
	require.Len(t, pings, 3)
	for _, p := range pings {
		assert.Equal(t, "192.0.2.1", p.SourceIP, "the forwarded headers stay ignored")
	}

	trusted, trustedLogs := newTestApp(t, func(c *Config) { c.TrustedProxies = "10.0.0.0/8" })
	hit(trusted, http.MethodGet, "/status/api", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "198.51.100.7") })
	assert.Empty(t, warnLines(trustedLogs, "MAINTENANT_TRUSTED_PROXIES"), "a declared proxy list does not warn")

	quiet, quietLogs := newTestApp(t, nil)
	hit(quiet, http.MethodGet, "/status/api", nil)
	assert.Empty(t, warnLines(quietLogs, "MAINTENANT_TRUSTED_PROXIES"), "no forwarded header, no warning")
}
