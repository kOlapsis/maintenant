// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func allowFrom(l *Limiter, remote string) bool {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	return l.AllowRequest(req)
}

func TestIPv6AddressesOfOneSlash64ShareABucket(t *testing.T) {
	l := New(0.0001, 1, NewClientIPResolver(nil))
	if !allowFrom(l, "[2001:db8:1:2::1]:4000") {
		t.Fatal("first request must pass")
	}
	if allowFrom(l, "[2001:db8:1:2:ffff:aaaa:bbbb:cccc]:4001") {
		t.Fatal("another address of the same /64 must hit the same bucket")
	}
	if !allowFrom(l, "[2001:db8:1:3::1]:4000") {
		t.Fatal("a different /64 must get its own bucket")
	}
}

func TestNeighbouringIPv4AddressesKeepSeparateBuckets(t *testing.T) {
	l := New(0.0001, 1, NewClientIPResolver(nil))
	if !allowFrom(l, "203.0.113.5:4000") || !allowFrom(l, "203.0.113.6:4000") {
		t.Fatal("two neighbouring IPv4 addresses must not share a bucket")
	}
	if allowFrom(l, "203.0.113.5:4001") {
		t.Fatal("the same IPv4 address must share its bucket")
	}
}

func TestForwardedIPv6ClientIsMaskedToo(t *testing.T) {
	trusted, err := ParsePrefixes("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	l := New(0.0001, 1, NewClientIPResolver(trusted))
	call := func(client string) bool {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:4000"
		req.Header.Set("X-Forwarded-For", client)
		return l.AllowRequest(req)
	}
	if !call("2001:db8:9:9::1") || call("2001:db8:9:9::2") {
		t.Fatal("forwarded IPv6 clients of one /64 must share a bucket")
	}
}

func TestVisitorTableStopsAtItsCap(t *testing.T) {
	l := New(0.0001, 1, NewClientIPResolver(nil))
	l.maxVisitors = 3
	for _, ip := range []string{"a", "b", "c"} {
		if !l.Allow(ip) {
			t.Fatalf("%s must be admitted below the cap", ip)
		}
	}
	if l.Allow("d") {
		t.Fatal("a new visitor must be refused once the table is full")
	}
	if n := l.Len(); n != 3 {
		t.Fatalf("table holds %d visitors, want 3", n)
	}
}

func TestVisitorTableReusesRoomFreedByIdleEviction(t *testing.T) {
	l := New(0.0001, 1, NewClientIPResolver(nil))
	l.maxVisitors = 2
	l.Allow("a")
	l.Allow("b")
	l.visitors.Range(func(_, v any) bool {
		vis := v.(*visitor)
		vis.lastRefil = vis.lastRefil.Add(-l.idleBeforeEviction() - 1)
		return true
	})
	if !l.Allow("c") {
		t.Fatal("idle visitors must be evicted to make room")
	}
	if n := l.Len(); n > 2 {
		t.Fatalf("table holds %d visitors, cap is 2", n)
	}
}
