// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 043-public-surfaces-hardening: the host names derived from the configuration.

// Edge case of US3: MAINTENANT_STATUS_URL gives a host name without port, and a value that does not parse refuses to start.
func TestConfig_StatusHost(t *testing.T) {
	valid := map[string]string{
		"":                                 "",
		"https://status.example.com":       "status.example.com",
		"https://Status.Example.com:8443/": "status.example.com",
		"http://status.lan/status/":        "status.lan",
		"https://now.example.com/status":   "now.example.com",
	}
	for in, want := range valid {
		got, err := Config{StatusURL: in}.StatusHost()
		require.NoError(t, err, "%q", in)
		assert.Equal(t, want, got, "%q", in)
		assert.NoError(t, Config{StatusURL: in}.ValidateHTTP(), "%q", in)
	}

	for _, in := range []string{"status.example.com", "status.example.com:8443", "ftp://status.example.com", "https://", "https:///status", "://nope", "not a url"} {
		_, err := Config{StatusURL: in}.StatusHost()
		require.Error(t, err, "%q", in)
		assert.Contains(t, err.Error(), "MAINTENANT_STATUS_URL", "%q: the message names the variable", in)

		err = Config{StatusURL: in}.ValidateHTTP()
		require.Error(t, err, "%q refuses to start", in)
		assert.Contains(t, err.Error(), "MAINTENANT_STATUS_URL", "%q", in)
	}
}

// FR-030 and its edge cases: the accepted set is derived from the configuration unless the variable is set;
// localhost, the loopback addresses and the listen host are always in.
func TestConfig_AllowedHostSet(t *testing.T) {
	has := func(t *testing.T, set map[string]struct{}, hosts ...string) {
		t.Helper()
		for _, h := range hosts {
			_, ok := set[h]
			assert.True(t, ok, "%q must be accepted, set is %v", h, set)
		}
	}
	lacks := func(t *testing.T, set map[string]struct{}, hosts ...string) {
		t.Helper()
		for _, h := range hosts {
			_, ok := set[h]
			assert.False(t, ok, "%q must not be accepted, set is %v", h, set)
		}
	}

	t.Run("derived from the configuration", func(t *testing.T) {
		set := Config{Addr: "192.168.1.5:8080", BaseURL: "https://Now.Example.com:8443", StatusURL: "https://status.example.com"}.AllowedHostSet()
		has(t, set, "localhost", "127.0.0.1", "::1", "192.168.1.5", "now.example.com", "status.example.com")
		lacks(t, set, "evil.example", "now.example.com:8443", "192.168.1.5:8080", "")
	})

	t.Run("wildcard listen addresses add nothing", func(t *testing.T) {
		for _, addr := range []string{"0.0.0.0:8080", "[::]:8080", ":8080", ""} {
			set := Config{Addr: addr, BaseURL: "http://" + addr}.AllowedHostSet()
			has(t, set, "localhost", "127.0.0.1", "::1")
			lacks(t, set, "0.0.0.0", "::", "")
		}
	})

	t.Run("explicit list replaces the derived hosts", func(t *testing.T) {
		set := Config{
			Addr: "127.0.0.1:8080", BaseURL: "https://app.example.com", StatusURL: "https://status.example.com",
			AllowedHosts: "now.example.com, Monitor.LAN:8443,, ",
		}.AllowedHostSet()
		has(t, set, "now.example.com", "monitor.lan", "localhost", "127.0.0.1", "::1")
		lacks(t, set, "app.example.com", "status.example.com", "monitor.lan:8443", "Monitor.LAN", "")
	})

	t.Run("a list without a valid entry falls back", func(t *testing.T) {
		set := Config{Addr: "127.0.0.1:8080", BaseURL: "https://app.example.com", AllowedHosts: " , ,"}.AllowedHostSet()
		has(t, set, "app.example.com", "localhost")
		lacks(t, set, "")
	})
}
