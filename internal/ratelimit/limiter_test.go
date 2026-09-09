// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A caller that renames itself on every request must not get a fresh bucket:
// with no trusted proxy the header is not read at all.
func TestMiddlewareSpoofedHeadersShareOneBucket(t *testing.T) {
	l := New(0.0001, 2, NewClientIPResolver(nil))
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	codes := make([]int, 0, 3)
	for _, spoofed := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		req := httptest.NewRequest(http.MethodGet, "/ping/x", nil)
		req.RemoteAddr = "203.0.113.5:44000"
		req.Header.Set("X-Real-IP", spoofed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}

	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("first two requests should pass, got %v", codes)
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Fatalf("third request should be rate limited, got %d", codes[2])
	}
}

// Behind a declared proxy the quota follows the forwarded client, so two
// distinct clients arriving through the same proxy keep separate buckets.
func TestMiddlewareCountsForwardedClientBehindTrustedProxy(t *testing.T) {
	trusted, err := ParsePrefixes("10.0.0.0/8")
	if err != nil {
		t.Fatalf("ParsePrefixes: %v", err)
	}
	l := New(0.0001, 1, NewClientIPResolver(trusted))
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func(client string) int {
		req := httptest.NewRequest(http.MethodGet, "/ping/x", nil)
		req.RemoteAddr = "10.0.0.1:44000"
		req.Header.Set("X-Forwarded-For", client)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call("198.51.100.7"); code != http.StatusOK {
		t.Fatalf("first client: got %d", code)
	}
	if code := call("203.0.113.9"); code != http.StatusOK {
		t.Fatalf("second client must have its own bucket: got %d", code)
	}
	if code := call("198.51.100.7"); code != http.StatusTooManyRequests {
		t.Fatalf("first client past its burst: got %d", code)
	}
}

func TestLimiterEvictionWaitsForARefilledBucket(t *testing.T) {
	tight := New(10, 20, nil)
	if got := tight.idleBeforeEviction(); got != 3*time.Minute {
		t.Fatalf("tight bucket: got %s, want the 3 minute floor", got)
	}

	hourly := New(5.0/3600.0, 5, nil)
	if got := hourly.idleBeforeEviction(); got != time.Hour {
		t.Fatalf("hourly bucket: got %s, want 1h", got)
	}
}
