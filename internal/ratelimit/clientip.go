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
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIPResolver turns a request into the address a quota is counted against.
type ClientIPResolver struct{ trusted []netip.Prefix }

// NewClientIPResolver returns a resolver that reads forwarded headers only from the given prefixes.
func NewClientIPResolver(trusted []netip.Prefix) *ClientIPResolver {
	return &ClientIPResolver{trusted: trusted}
}

// Resolve returns the address the request is attributed to.
func (r *ClientIPResolver) Resolve(req *http.Request) netip.Addr {
	peer := peerAddr(req.RemoteAddr)
	if !r.TrustsPeer(req) {
		return peer
	}

	if fwd := req.Header.Get("X-Forwarded-For"); fwd != "" {
		hops := strings.Split(fwd, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				continue
			}
			if addr = addr.Unmap(); !r.trusts(addr) {
				return addr
			}
		}
		return peer
	}

	if real := req.Header.Get("X-Real-IP"); real != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(real)); err == nil {
			return addr.Unmap()
		}
	}
	return peer
}

// TrustsPeer reports whether the request comes straight from a configured trusted proxy.
func (r *ClientIPResolver) TrustsPeer(req *http.Request) bool {
	if r == nil || len(r.trusted) == 0 || req == nil {
		return false
	}
	return r.trusts(peerAddr(req.RemoteAddr))
}

// ClientIP returns the resolved address as the string key a quota is stored under.
func (r *ClientIPResolver) ClientIP(req *http.Request) string {
	if addr := r.Resolve(req); addr.IsValid() {
		return addr.String()
	}
	return req.RemoteAddr
}

func (r *ClientIPResolver) trusts(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, p := range r.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func peerAddr(remoteAddr string) netip.Addr {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// ParsePrefixes reads a comma-separated list of CIDRs and bare IP addresses.
func ParsePrefixes(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			p, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, err
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, err
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}
