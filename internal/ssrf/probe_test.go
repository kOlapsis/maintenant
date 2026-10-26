// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package ssrf

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C7: the destination rule of label-declared monitors.

// FR-012: link-local, multicast and unspecified are refused; private, ULA, CGNAT and loopback are not.
func TestIsBlockedForProbe(t *testing.T) {
	for _, s := range []string{
		"169.254.169.254", "169.254.0.1", "169.254.255.254",
		"fe80::1", "fe80::dead:beef",
		"224.0.0.1", "239.255.255.250", "ff02::1",
		"0.0.0.0", "::",
	} {
		assert.Truef(t, IsBlockedForProbe(net.ParseIP(s)), "%s must be refused", s)
	}
	assert.True(t, IsBlockedForProbe(nil), "an unparsable address is refused")

	for _, s := range []string{
		"127.0.0.1", "127.1.2.3", "::1",
		"10.0.0.1", "172.16.5.4", "192.168.1.1", "fd00::1",
		"100.64.0.1",
		"8.8.8.8", "2001:4860:4860::8888",
	} {
		assert.Falsef(t, IsBlockedForProbe(net.ParseIP(s)), "%s must stay reachable", s)
	}
}

// FR-012: the dial hook names the refused address in its error and lets the others through.
func TestProbeControl(t *testing.T) {
	err := ProbeControl("tcp", "169.254.169.254:80", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe guard: blocked connection to 169.254.169.254")

	err = ProbeControl("tcp", "[fe80::1]:443", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe guard: blocked connection to fe80::1")

	assert.NoError(t, ProbeControl("tcp", "10.0.0.1:80", nil))
	assert.NoError(t, ProbeControl("tcp", "127.0.0.1:8080", nil))
	assert.NoError(t, ProbeControl("tcp", "[::1]:8080", nil))
	assert.Error(t, ProbeControl("tcp", "not-an-address", nil))
}

// FR-013: the opt-out is read from the environment and only a true value lifts the guard.
func TestProbeGuardDisabled(t *testing.T) {
	t.Setenv("MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL", "")
	assert.False(t, ProbeGuardDisabled())

	t.Setenv("MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL", "true")
	assert.True(t, ProbeGuardDisabled())

	t.Setenv("MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL", "false")
	assert.False(t, ProbeGuardDisabled())

	t.Setenv("MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL", "no")
	assert.False(t, ProbeGuardDisabled())
}
