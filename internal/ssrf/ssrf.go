// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

// Package ssrf guards outbound webhook and notification delivery against
// server-side request forgery: a user-supplied URL that points at loopback,
// the LAN, or a cloud metadata endpoint (169.254.169.254). It offers a
// create-time URL check for fast feedback and a dial-time Control hook that is
// the actual security boundary — the hook runs after DNS resolution on the
// concrete address being dialed, so it also defeats DNS rebinding.
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a URL resolves to, or a connection targets,
// an internal/private IP that must not be reachable as a delivery destination.
var ErrBlockedAddress = errors.New("URL must not resolve to a private or internal IP address")

// IsBlocked reports whether ip must not be used as a delivery destination. It
// rejects loopback, link-local, private (RFC 1918 / ULA), CGNAT (RFC 6598),
// unspecified, and multicast addresses — the ranges an SSRF payload would use
// to reach cloud metadata, localhost, or the LAN.
func IsBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// CGNAT / carrier-grade NAT: 100.64.0.0/10 (RFC 6598).
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// ValidateURL fails fast at create time: it requires an https scheme and rejects
// hostnames that resolve to a blocked IP. It is a fast-feedback guard — the
// dial-time Control hook is the true security boundary (it also defeats DNS
// rebinding between validation and delivery).
func ValidateURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL format")
	}
	if parsed.Scheme != "https" {
		return errors.New("webhook URL must use https scheme")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("webhook URL must include a host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsBlocked(ip) {
			return ErrBlockedAddress
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("cannot resolve webhook hostname: %s", host)
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && IsBlocked(ip) {
			return ErrBlockedAddress
		}
	}
	return nil
}

// control is a net.Dialer.Control hook that rejects a connection whose resolved
// address is a blocked IP. It runs after DNS resolution, on the concrete address
// being dialed, so it holds even when a hostname rebinds to an internal IP
// between validation and delivery, and on every redirect hop.
func control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf guard: cannot parse dial address %q", address)
	}
	if IsBlocked(ip) {
		return fmt.Errorf("ssrf guard: blocked connection to %s: %w", ip, ErrBlockedAddress)
	}
	return nil
}

// NewHTTPClient returns an http.Client whose dialer blocks connections to
// internal/private IPs on every hop, including redirects. When allowPrivate is
// true (dev only, via MAINTENANT_ALLOW_PRIVATE_WEBHOOKS) the guard is disabled.
func NewHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !allowPrivate {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second, Control: control}
		transport.DialContext = dialer.DialContext
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
