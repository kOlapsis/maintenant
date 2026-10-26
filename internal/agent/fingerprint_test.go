// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// Acceptance tests of 046-agent-grpc-hardening, agent side (US1, FR-004, FR-005):
// --grpc-server-fingerprint pins the server certificate; --grpc-insecure-skip-tls-verify
// warns on every connection.

// serverCert returns a self-signed certificate for a name that is not the one dialed.
func serverCert(t *testing.T, cn string) (*tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	sum := sha256.Sum256(der)
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, hex.EncodeToString(sum[:])
}

type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

// text flattens a record, message and attributes, for substring checks.
func (h *recordingHandler) text(r slog.Record) string {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" " + a.Key + "=" + a.Value.String())
		return true
	})
	return b.String()
}

func (h *recordingHandler) all() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var lines []string
	for _, r := range h.records {
		lines = append(lines, r.Level.String()+" "+h.text(r))
	}
	return strings.Join(lines, "\n")
}

// skipVerifyWarnings counts the WARN records about disabled certificate verification.
func (h *recordingHandler) skipVerifyWarnings() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		txt := strings.ToLower(h.text(r))
		if strings.Contains(txt, "verif") || strings.Contains(txt, "insecure") || strings.Contains(txt, "skip") {
			n++
		}
	}
	return n
}

func recordingLogger() (*slog.Logger, *recordingHandler) {
	h := &recordingHandler{}
	return slog.New(h), h
}

// serveIngestAt serves srv over TLS on addr and counts the handshakes it completes.
func serveIngestAt(t *testing.T, addr string, srv agentpb.IngestServer, cert *tls.Certificate) (handshakes *atomic.Int32, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	handshakes = &atomic.Int32{}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
		VerifyConnection: func(tls.ConnectionState) error {
			handshakes.Add(1)
			return nil
		},
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	agentpb.RegisterIngestServer(s, srv)
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return handshakes, s.Stop
}

func registerAs(ctx context.Context, c *Client, id, token string) error {
	_, err := c.Register(ctx, &agentpb.RegisterRequest{AgentId: id, EnrollmentToken: token, PublicKey: make([]byte, 32)})
	return err
}

// US1 scenarios 4 and 5, FR-004, SC-002: the matching fingerprint is accepted without a
// trust chain or a matching host name; any other fingerprint is refused before the
// first RPC, and the log names both fingerprints.
func TestNewClientWithFingerprint_AcceptsOnlyTheMatchingCertificate(t *testing.T) {
	cert, served := serverCert(t, "elsewhere.invalid")
	_, other := serverCert(t, "another.invalid")
	ingest := &fakeIngest{validToken: "tok"}
	url := startIngest(t, ingest, cert)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("no pin and no trust: refused (guard)", func(t *testing.T) {
		c := dialIngest(t, url)
		require.Error(t, registerAs(ctx, c, "guard", "tok"), "the test certificate must not be trusted by itself")
	})

	t.Run("matching fingerprint: accepted", func(t *testing.T) {
		logger, _ := recordingLogger()
		c, err := NewClientWithFingerprint(ctx, url, served, false, logger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
		require.NoError(t, registerAs(ctx, c, "pinned-ok", "tok"))
		assert.Contains(t, ingest.enrolledIDs(), "pinned-ok")
	})

	t.Run("different fingerprint: refused before any RPC", func(t *testing.T) {
		logger, logs := recordingLogger()
		c, err := NewClientWithFingerprint(ctx, url, other, false, logger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })

		shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = registerAs(shortCtx, c, "pinned-wrong", "tok")
		require.Error(t, err, "a server whose certificate does not match the pin is refused")
		assert.NotContains(t, ingest.enrolledIDs(), "pinned-wrong", "the RPC must never reach the server")

		out := logs.all()
		assert.Contains(t, out, served, "the log names the fingerprint received")
		assert.Contains(t, out, other, "the log names the fingerprint expected")
	})
}

// Edge case "empreinte avec des deux-points ou en majuscules": accepted after normalisation.
func TestNewClientWithFingerprint_NormalisesTheGivenFingerprint(t *testing.T) {
	cert, served := serverCert(t, "elsewhere.invalid")
	ingest := &fakeIngest{validToken: "tok"}
	url := startIngest(t, ingest, cert)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	upper := strings.ToUpper(served)
	var colons []string
	for i := 0; i < len(upper); i += 2 {
		colons = append(colons, upper[i:i+2])
	}
	for name, given := range map[string]string{
		"uppercase":             upper,
		"uppercase with colons": strings.Join(colons, ":"),
		"lowercase with colons": strings.ToLower(strings.Join(colons, ":")),
	} {
		t.Run(name, func(t *testing.T) {
			logger, _ := recordingLogger()
			c, err := NewClientWithFingerprint(ctx, url, given, false, logger)
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close() })
			require.NoError(t, registerAs(ctx, c, "norm-"+name, "tok"))
			assert.Contains(t, ingest.enrolledIDs(), "norm-"+name)
		})
	}
}

// Contract "Agent": a fingerprint that is not 64 hex digits, or given with a plaintext
// server URL, refuses to start.
func TestNewClientWithFingerprint_RefusesMalformedOrPlaintext(t *testing.T) {
	_, served := serverCert(t, "elsewhere.invalid")
	logger, _ := recordingLogger()
	for name, c := range map[string]struct{ url, fp string }{
		"too short":        {"grpcs://127.0.0.1:1", served[:63]},
		"too long":         {"grpcs://127.0.0.1:1", served + "0"},
		"not hex":          {"grpcs://127.0.0.1:1", strings.Repeat("zz", 32)},
		"plaintext server": {"grpc://127.0.0.1:1", served},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClientWithFingerprint(context.Background(), c.url, c.fp, false, logger)
			require.Error(t, err, "the agent must refuse to start")
			if client != nil {
				_ = client.Close()
			}
		})
	}
}

// US1 scenario 6, FR-005, edge case "empreinte et skip-verify ensemble": the workaround
// warns on every connection, reconnections included, and never weakens a given pin.
func TestNewClientWithFingerprint_WarnsOnEveryConnectionWhenVerificationIsSkipped(t *testing.T) {
	cert1, fp1 := serverCert(t, "first.invalid")
	cert2, fp2 := serverCert(t, "second.invalid")
	ingest := &fakeIngest{validToken: "tok"}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	url := "grpcs://" + addr

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handshakes1, stop1 := serveIngestAt(t, addr, ingest, cert1)
	logger, logs := recordingLogger()
	c, err := NewClientWithFingerprint(ctx, url, "", true, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, registerAs(ctx, c, "skip-1", "tok"))
	require.EqualValues(t, 1, handshakes1.Load())
	first := logs.skipVerifyWarnings()
	assert.GreaterOrEqual(t, first, 1, "the first connection warns:\n%s", logs.all())

	// The server goes away and comes back with another certificate: the client reconnects.
	stop1()
	handshakes2, _ := serveIngestAt(t, addr, ingest, cert2)
	require.Eventually(t, func() bool {
		return registerAs(ctx, c, "skip-2", "tok") == nil
	}, 20*time.Second, 200*time.Millisecond, "the client must reconnect to the restarted server")
	require.GreaterOrEqual(t, handshakes2.Load(), int32(1))
	assert.GreaterOrEqual(t, logs.skipVerifyWarnings(), first+1, "the reconnection warns again:\n%s", logs.all())

	t.Run("pin and skip together: the pin still decides", func(t *testing.T) {
		okLogger, okLogs := recordingLogger()
		both, err := NewClientWithFingerprint(ctx, url, fp2, true, okLogger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = both.Close() })
		require.NoError(t, registerAs(ctx, both, "both-ok", "tok"))
		assert.GreaterOrEqual(t, okLogs.skipVerifyWarnings(), 1, "the workaround warning is written even with a pin")

		wrongLogger, _ := recordingLogger()
		wrong, err := NewClientWithFingerprint(ctx, url, fp1, true, wrongLogger)
		require.NoError(t, err)
		t.Cleanup(func() { _ = wrong.Close() })
		shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		require.Error(t, registerAs(shortCtx, wrong, "both-wrong", "tok"), "skip-verify must not bypass a mismatching pin")
		assert.NotContains(t, ingest.enrolledIDs(), "both-wrong")
	})
}
