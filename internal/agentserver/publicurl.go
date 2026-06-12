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

package agentserver

import (
	"net"
	"net/http"
	"strings"
)

// PublicURLConfig holds the inputs for resolving the gRPC public URL.
type PublicURLConfig struct {
	// Explicit override from MAINTENANT_GRPC_URL env or --grpc-url flag.
	// If non-empty, it is used as-is (after ensuring the grpcs:// scheme).
	Explicit string

	// ListenAddr is the address the gRPC server is bound to (e.g. "127.0.0.1:8443").
	// Used as fallback when neither Explicit nor request headers are available.
	ListenAddr string
}

// ResolvePublicURL returns the grpcs:// URL that remote agents should use plus a list
// of warnings. Resolution priority: explicit config > X-Forwarded-Host+Proto headers
// from the HTTP request > request Host header.
//
// A "public_url_appears_local" warning is appended whenever the resolved host
// is localhost, 127.x.x.x, ::1, or an RFC-1918 / link-local address.
func ResolvePublicURL(req *http.Request, cfg PublicURLConfig) (string, []string) {
	var warnings []string

	// 1. Explicit env / flag override.
	if cfg.Explicit != "" {
		url := ensureGRPCSScheme(cfg.Explicit)
		if looksLocal(hostFromURL(url)) {
			warnings = append(warnings, "public_url_appears_local")
		}
		return url, warnings
	}

	// 2. X-Forwarded-Host + X-Forwarded-Proto headers (reverse-proxy path).
	if req != nil {
		fwdHost := req.Header.Get("X-Forwarded-Host")
		fwdProto := req.Header.Get("X-Forwarded-Proto")
		if fwdHost != "" {
			// Normalise: remove standard gRPC port 443 suffix.
			host := stripStandardPort(fwdHost, "443")
			var url string
			if fwdProto == "grpc" {
				url = "grpc://" + host
			} else {
				url = "grpcs://" + host
			}
			if looksLocal(fwdHost) {
				warnings = append(warnings, "public_url_appears_local")
			}
			return url, warnings
		}

		// 3. Request Host header.
		if req.Host != "" {
			host := stripStandardPort(req.Host, "443")
			url := "grpcs://" + host
			if looksLocal(req.Host) {
				warnings = append(warnings, "public_url_appears_local")
			}
			return url, warnings
		}
	}

	// 4. Fallback to configured listen address (almost certainly local).
	listen := cfg.ListenAddr
	if listen == "" {
		listen = "127.0.0.1:8443"
	}
	url := "grpcs://" + listen
	warnings = append(warnings, "public_url_appears_local")
	return url, warnings
}

func ensureGRPCSScheme(u string) string {
	if strings.HasPrefix(u, "grpcs://") || strings.HasPrefix(u, "grpc://") {
		return u
	}
	return "grpcs://" + strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
}

func hostFromURL(u string) string {
	for _, prefix := range []string{"grpcs://", "grpc://", "https://", "http://"} {
		if strings.HasPrefix(u, prefix) {
			rest := strings.TrimPrefix(u, prefix)
			// Strip path/port for the local check
			host, _, _ := net.SplitHostPort(rest)
			if host != "" {
				return host
			}
			return rest
		}
	}
	return u
}

// looksLocal returns true when h resolves to a loopback, link-local, or RFC-1918 address.
func looksLocal(h string) bool {
	// Strip port if present.
	host := h
	if hh, _, err := net.SplitHostPort(h); err == nil {
		host = hh
	}

	ip := net.ParseIP(host)
	if ip == nil {
		lower := strings.ToLower(host)
		return lower == "localhost" || strings.HasSuffix(lower, ".local")
	}

	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}

func stripStandardPort(host, standard string) string {
	if _, port, err := net.SplitHostPort(host); err == nil && port == standard {
		h, _, _ := net.SplitHostPort(host)
		return h
	}
	return host
}
