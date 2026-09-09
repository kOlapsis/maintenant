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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
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
// surrender its certificate chain so expiry monitoring keeps working. The probe
// never gets far enough to learn an HTTP status: it stops at the handshake.
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
	assert.Nil(t, result.HTTPStatus, "a handshake-only probe never learns an HTTP status")
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

// An untrusted certificate is reported as degraded regardless of what the
// application behind it would have answered: the request is never sent, so a
// 500 the handler is ready to return is never observed.
func TestCheckHTTP_UntrustedCertificate_AppStatusNeverObserved(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	result := CheckHTTP(context.Background(), httpEndpoint(srv.URL), nil)

	assert.True(t, result.Success, "the handshake succeeded; the never-sent request cannot fail it")
	assert.True(t, result.Degraded)
	assert.Nil(t, result.HTTPStatus)
	assert.Contains(t, result.ErrorMessage, "unknown authority")
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

// recordingServer starts a TLS test server that records every call it
// receives, along with the headers it saw.
func recordingServer(t *testing.T) (*httptest.Server, func() int, func() []http.Header) {
	t.Helper()

	var mu sync.Mutex
	var calls int
	var headers []http.Header

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		headers = append(headers, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	callCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
	seenHeaders := func() []http.Header {
		mu.Lock()
		defer mu.Unlock()
		return headers
	}
	return srv, callCount, seenHeaders
}

// A rejected certificate must never be followed by the actual request: the
// secrets it carries (Authorization, API keys, the URL itself) must not reach
// a peer whose identity was never verified.
func TestCheckHTTP_TrustFailure_NeverSendsTheRequest(t *testing.T) {
	srv, callCount, _ := recordingServer(t)

	ep := httpEndpoint(srv.URL + "/probe?token=s")
	ep.Config.TLSVerify = true
	ep.Config.Headers = map[string]string{
		"Authorization": "Bearer audit-fixture",
		"X-Api-Key":     "k",
	}

	result := CheckHTTP(context.Background(), ep, nil)

	assert.Equal(t, 0, callCount(), "the request must never be sent to an unverified peer")
	assert.True(t, result.Degraded)
	assert.True(t, result.Success)
	assert.Nil(t, result.HTTPStatus, "a handshake-only probe carries no HTTP status")
	assert.NotEmpty(t, result.TLSPeerCertificates)
	assert.NotEmpty(t, result.DegradedReason)
}

// Same guarantee for a method with a body-bearing semantics: it must not be
// replayed either.
func TestCheckHTTP_TrustFailure_POSTIsNotReplayedEither(t *testing.T) {
	srv, callCount, _ := recordingServer(t)

	ep := httpEndpoint(srv.URL + "/probe?token=s")
	ep.Config.TLSVerify = true
	ep.Config.Method = "POST"
	ep.Config.Headers = map[string]string{
		"Authorization": "Bearer audit-fixture",
	}

	CheckHTTP(context.Background(), ep, nil)

	assert.Equal(t, 0, callCount())
}

// With verification switched off per endpoint, the request is sent as before
// (once), headers included.
func TestCheckHTTP_TLSVerifyOff_SendsHeadersOnce(t *testing.T) {
	srv, callCount, seenHeaders := recordingServer(t)

	ep := httpEndpoint(srv.URL)
	ep.Config.TLSVerify = false
	ep.Config.Headers = map[string]string{
		"Authorization": "Bearer audit-fixture",
	}

	result := CheckHTTP(context.Background(), ep, nil)

	require.True(t, result.Success)
	assert.Equal(t, 1, callCount())
	require.Len(t, seenHeaders(), 1)
	assert.Equal(t, "Bearer audit-fixture", seenHeaders()[0].Get("Authorization"))
}

// A host whose handshake fails outright (no certificate to report at all) is
// down, not degraded: the no-retry path must not paper over it.
func TestCheckHTTP_TrustFailureThenNoTLS_IsDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	ep := httpEndpoint(fmt.Sprintf("https://%s/", ln.Addr().String()))

	result := CheckHTTP(context.Background(), ep, nil)

	assert.False(t, result.Success)
	assert.False(t, result.Degraded)
	assert.NotEmpty(t, result.ErrorMessage)
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
