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
	"net/netip"
	"testing"
)

func request(remoteAddr string, headers map[string]string) *http.Request {
	req := &http.Request{
		RemoteAddr: remoteAddr,
		Header:     http.Header{},
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func prefixes(t *testing.T, list string) []netip.Prefix {
	t.Helper()
	p, err := ParsePrefixes(list)
	if err != nil {
		t.Fatalf("ParsePrefixes(%q): %v", list, err)
	}
	return p
}

func TestResolveIgnoresHeadersWithoutTrustedProxies(t *testing.T) {
	r := NewClientIPResolver(nil)
	req := request("203.0.113.5:44000", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
		"X-Real-IP":       "5.6.7.8",
	})

	if got := r.Resolve(req).String(); got != "203.0.113.5" {
		t.Fatalf("got %s, want the peer address", got)
	}
}

func TestResolveTakesLastUntrustedForwardedHop(t *testing.T) {
	r := NewClientIPResolver(prefixes(t, "10.0.0.0/8"))
	req := request("10.0.0.1:44000", map[string]string{
		"X-Forwarded-For": "198.51.100.7, 10.0.0.9",
	})

	if got := r.Resolve(req).String(); got != "198.51.100.7" {
		t.Fatalf("got %s, want 198.51.100.7", got)
	}
}

func TestResolveIgnoresForgedHeaderFromUntrustedPeer(t *testing.T) {
	r := NewClientIPResolver(prefixes(t, "10.0.0.0/8"))
	req := request("203.0.113.5:44000", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
	})

	if got := r.Resolve(req).String(); got != "203.0.113.5" {
		t.Fatalf("got %s, want the peer address", got)
	}
}

func TestResolveRealIPOnlyWhenForwardedForAbsent(t *testing.T) {
	r := NewClientIPResolver(prefixes(t, "10.0.0.0/8"))

	behind := request("10.0.0.1:44000", map[string]string{"X-Real-IP": "198.51.100.7"})
	if got := r.Resolve(behind).String(); got != "198.51.100.7" {
		t.Fatalf("trusted peer: got %s, want 198.51.100.7", got)
	}

	withXFF := request("10.0.0.1:44000", map[string]string{
		"X-Forwarded-For": "203.0.113.9",
		"X-Real-IP":       "198.51.100.7",
	})
	if got := r.Resolve(withXFF).String(); got != "203.0.113.9" {
		t.Fatalf("X-Forwarded-For wins: got %s, want 203.0.113.9", got)
	}

	direct := request("203.0.113.5:44000", map[string]string{"X-Real-IP": "198.51.100.7"})
	if got := r.Resolve(direct).String(); got != "203.0.113.5" {
		t.Fatalf("untrusted peer: got %s, want the peer address", got)
	}
}

func TestResolveIPv6Peer(t *testing.T) {
	r := NewClientIPResolver(nil)

	bracketed := request("[2001:db8::1]:44000", nil)
	if got := r.Resolve(bracketed).String(); got != "2001:db8::1" {
		t.Fatalf("bracketed: got %s", got)
	}

	bare := request("2001:db8::1", nil)
	if got := r.Resolve(bare).String(); got != "2001:db8::1" {
		t.Fatalf("bare: got %s", got)
	}
}

func TestResolveIPv6TrustedProxy(t *testing.T) {
	r := NewClientIPResolver(prefixes(t, "2001:db8::/32"))
	req := request("[2001:db8::1]:44000", map[string]string{
		"X-Forwarded-For": "198.51.100.7, 2001:db8::5",
	})

	if got := r.Resolve(req).String(); got != "198.51.100.7" {
		t.Fatalf("got %s, want 198.51.100.7", got)
	}
}

func TestResolveGarbageHeaderFallsBackToPeer(t *testing.T) {
	r := NewClientIPResolver(prefixes(t, "10.0.0.0/8"))

	garbageXFF := request("10.0.0.1:44000", map[string]string{"X-Forwarded-For": "not-an-ip, ../../etc"})
	if got := r.Resolve(garbageXFF).String(); got != "10.0.0.1" {
		t.Fatalf("garbage X-Forwarded-For: got %s", got)
	}

	garbageReal := request("10.0.0.1:44000", map[string]string{"X-Real-IP": "not-an-ip"})
	if got := r.Resolve(garbageReal).String(); got != "10.0.0.1" {
		t.Fatalf("garbage X-Real-IP: got %s", got)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := NewClientIPResolver(nil)
	if got := r.ClientIP(request("unix-socket", nil)); got != "unix-socket" {
		t.Fatalf("got %s, want the raw RemoteAddr", got)
	}
}

func TestParsePrefixesAcceptsBareAddressesAndCIDRs(t *testing.T) {
	got := prefixes(t, " 10.0.0.0/8 , 192.168.1.4 ,, 2001:db8::/32 ")
	want := []string{"10.0.0.0/8", "192.168.1.4/32", "2001:db8::/32"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, p := range got {
		if p.String() != want[i] {
			t.Fatalf("prefix %d: got %s, want %s", i, p, want[i])
		}
	}
}

func TestParsePrefixesRefusesGarbage(t *testing.T) {
	if _, err := ParsePrefixes("10.0.0.0/8,nonsense"); err == nil {
		t.Fatal("expected an error")
	}
}
