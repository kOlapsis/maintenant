// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURL(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"userinfo":        {"https://ops:hunter2@example.com/health", "https://example.com"},
		"query token":     {"https://example.com/health?token=t0k3n", "https://example.com"},
		"path secret":     {"https://api.example.com/hooks/S3CR3T/ping", "https://api.example.com"},
		"port kept":       {"http://example.com:8080/x?y=1#frag", "http://example.com:8080"},
		"ipv6 host":       {"https://admin:pw@[2001:db8::1]:8443/p?k=v", "https://[2001:db8::1]:8443"},
		"bare host":       {"https://example.com", "https://example.com"},
		"host and port":   {"db.internal:5432", "db.internal:5432"},
		"ipv6 and port":   {"[::1]:5432", "[::1]:5432"},
		"email":           {"ops@example.com", "ops@example.com"},
		"telegram chat":   {"-1001234567890", "-1001234567890"},
		"unreadable url":  {"https://ops:hunter%zz@example.com/p?token=t0k3n", "https://"},
		"other scheme":    {"tcp://user:pw@db.internal:5432/x", "tcp://db.internal:5432"},
		"empty":           {"", ""},
		"upper case http": {"HTTPS://Example.com/Secret", "https://Example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, URL(tc.in))
		})
	}
}

func TestText(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"go http error": {
			`request failed: Get "https://ops:***@example.com/health?token=t0k3n": context deadline exceeded`,
			`request failed: Get "https://example.com": context deadline exceeded`,
		},
		"redirect in the error": {
			`request failed: Get "https://a.example.com/in?k=1": stopped after 3 redirects to https://b.example.com/out/S3CR3T`,
			`request failed: Get "https://a.example.com": stopped after 3 redirects to https://b.example.com`,
		},
		"alert message": {
			"Endpoint https://example.com/hooks/S3CR3T failed 3 consecutive checks",
			"Endpoint https://example.com failed 3 consecutive checks",
		},
		"json details": {
			`{"target":"https://u:p@example.com/x?token=t0k3n","last_error":"Get \"https://example.com/x?token=t0k3n\": EOF"}`,
			`{"target":"https://example.com","last_error":"Get \"https://example.com\": EOF"}`,
		},
		"ipv6": {
			`dial https://[2001:db8::1]:8443/p?k=v failed`,
			`dial https://[2001:db8::1]:8443 failed`,
		},
		"tcp error untouched": {
			"tcp dial failed: dial tcp 10.0.0.7:5432: connect: connection refused",
			"tcp dial failed: dial tcp 10.0.0.7:5432: connect: connection refused",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Text(tc.in))
		})
	}
}
