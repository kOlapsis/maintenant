// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license.

package ssrf

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlocked(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"169.254.169.254", "fe80::1", // link-local (incl. cloud IMDS)
		"10.0.0.1", "172.16.5.4", "192.168.1.1", "fd00::1", // private / ULA
		"100.64.0.1", "100.127.255.254", // CGNAT (RFC 6598)
		"0.0.0.0", "::", // unspecified
		"224.0.0.1", // multicast
	}
	for _, s := range blocked {
		assert.Truef(t, IsBlocked(net.ParseIP(s)), "%s should be blocked", s)
	}

	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34",
		"2001:4860:4860::8888",
		"100.63.255.255", "100.128.0.0", // just outside CGNAT range
	}
	for _, s := range allowed {
		assert.Falsef(t, IsBlocked(net.ParseIP(s)), "%s should be allowed", s)
	}

	assert.True(t, IsBlocked(nil), "nil IP must be blocked")
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()

	rejected := []string{
		"http://example.com/",                    // not https
		"ftp://example.com/",                     // not https
		"https://127.0.0.1/",                     // loopback literal
		"https://[::1]/",                         // loopback literal v6
		"https://169.254.169.254/latest/meta-data/", // cloud IMDS
		"https://10.0.0.5/hook",                  // private literal
		"https://localhost/hook",                 // resolves to loopback
		"https:///nohost",                        // no host
		"",                                       // empty
	}
	for _, u := range rejected {
		assert.Errorf(t, ValidateURL(ctx, u), "%q should be rejected", u)
	}

	allowed := []string{
		"https://8.8.8.8/hook",
		"https://1.1.1.1/",
	}
	for _, u := range allowed {
		assert.NoErrorf(t, ValidateURL(ctx, u), "%q should be allowed", u)
	}
}

func TestNewHTTPClient_BlocksLoopbackAtDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Guard enabled: dialing the loopback test server must fail.
	guarded := NewHTTPClient(2*time.Second, false)
	_, err := guarded.Get(srv.URL)
	require.Error(t, err, "guarded client must refuse to dial a loopback address")
	assert.Contains(t, err.Error(), "ssrf guard")

	// Guard disabled (dev mode): the same request must succeed.
	open := NewHTTPClient(2*time.Second, true)
	resp, err := open.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
