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

package endpoint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust"
)

func httpEndpoint(target string) *Endpoint {
	return &Endpoint{
		ID:           "ep-tls",
		EndpointType: TypeHTTP,
		Target:       target,
		Config:       DefaultConfig(),
	}
}

// Issue #36: a host answering over a certificate signed by an unknown authority
// is not down. It must report as degraded, keep counting as a success, and still
// surrender its certificate chain so expiry monitoring keeps working.
func TestCheckHTTP_UntrustedCertificateIsDegradedNotDown(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// httptest's certificate is signed by an authority nothing trusts, which is
	// exactly the reporter's internal-PKI situation.
	result := CheckHTTP(context.Background(), httpEndpoint(srv.URL), nil)

	assert.True(t, result.Success, "a reachable host must not count as a failure")
	assert.True(t, result.Degraded, "an untrusted certificate must mark the check degraded")
	assert.Contains(t, result.DegradedReason, "unknown authority")
	assert.Contains(t, result.ErrorMessage, "unknown authority",
		"the reason must reach the UI through last_error")
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusOK, *result.HTTPStatus)
	assert.NotEmpty(t, result.TLSPeerCertificates,
		"the chain must still be captured, or expiry monitoring silently stops for these hosts")
}

// The same host, trusted: no retry, no degradation.
func TestCheckHTTP_TrustedCertificateIsUp(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Trust the test server's own certificate the way an operator would: through
	// MAINTENANT_CA_CERT, exercising the real load path.
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	require.NoError(t, trust.Load(caPath))
	t.Cleanup(func() { _ = trust.Load("") })

	result := CheckHTTP(context.Background(), httpEndpoint(srv.URL), nil)

	assert.True(t, result.Success)
	assert.False(t, result.Degraded, "a trusted chain must not be reported as degraded")
	assert.Empty(t, result.ErrorMessage)
}

// A genuinely unreachable host stays down — the retry must not paper over it.
func TestCheckHTTP_UnreachableHostStaysDown(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	target := srv.URL
	srv.Close() // nothing is listening any more

	result := CheckHTTP(context.Background(), httpEndpoint(target), nil)

	assert.False(t, result.Success)
	assert.False(t, result.Degraded, "a connection failure is not a trust problem")
	assert.Contains(t, result.ErrorMessage, "request failed")
}

// An untrusted certificate on a host that also answers badly is still down: the
// status check governs, and the trust problem is reported alongside it.
func TestCheckHTTP_UntrustedAndBadStatusIsDown(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	result := CheckHTTP(context.Background(), httpEndpoint(srv.URL), nil)

	assert.False(t, result.Success, "a 500 is a failure regardless of the certificate")
	assert.Contains(t, result.ErrorMessage, "unexpected status 500")
	assert.Contains(t, result.ErrorMessage, "unknown authority",
		"both problems must be visible, not just the first")
}

// With verification switched off per endpoint, nothing is degraded.
func TestCheckHTTP_TLSVerifyDisabledIsPlainUp(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ep := httpEndpoint(srv.URL)
	ep.Config.TLSVerify = false

	result := CheckHTTP(context.Background(), ep, nil)

	assert.True(t, result.Success)
	assert.False(t, result.Degraded)
}

func TestTrustFailureReason(t *testing.T) {
	reason, ok := trustFailureReason(x509.UnknownAuthorityError{})
	assert.True(t, ok)
	assert.Contains(t, reason, "unknown authority")

	reason, ok = trustFailureReason(x509.HostnameError{Host: "wrong.example.com"})
	assert.True(t, ok)
	assert.Contains(t, reason, "hostname")

	reason, ok = trustFailureReason(x509.CertificateInvalidError{Reason: x509.Expired})
	assert.True(t, ok)
	assert.Contains(t, reason, "expired")

	// Wrapped the way crypto/tls and net/http actually deliver it.
	wrapped := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	reason, ok = trustFailureReason(wrapped)
	assert.True(t, ok)
	assert.Contains(t, reason, "unknown authority", "must unwrap to the specific cause")

	_, ok = trustFailureReason(errors.New("connection refused"))
	assert.False(t, ok, "a transport error is not a trust failure")

	_, ok = trustFailureReason(nil)
	assert.False(t, ok)
}

func TestCheckHTTP_DegradedResolvesToDegradedStatus(t *testing.T) {
	// Guards the classification itself, independently of the probe.
	cases := []struct {
		name     string
		result   CheckResult
		expected EndpointStatus
	}{
		{"reachable and trusted", CheckResult{Success: true}, StatusUp},
		{"reachable but untrusted", CheckResult{Success: true, Degraded: true}, StatusDegraded},
		{"unreachable", CheckResult{}, StatusDown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, statusForResult(tc.result))
		})
	}
}
