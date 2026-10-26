// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

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
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial"
	"github.com/kolapsis/maintenant/internal/extension"
)

// Acceptance tests of 046-agent-grpc-hardening through the whole server: the
// certificate lives next to the database, the token creation route reports it,
// and the embedded agent pins it.

func proEdition(t *testing.T) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func loopbackPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// multihostServer builds a server-mode app that serves agents over TLS, starts it and
// waits for the gRPC listener. The returned stop function shuts it down.
func multihostServer(t *testing.T, cfg *Config, logger *slog.Logger) (*App, func()) {
	t.Helper()
	cfg.Mode = "server"
	cfg.Addr = loopbackPort(t)
	cfg.MultiHost.GRPCListen = loopbackPort(t)
	a, err := New(*cfg, logger, WithExtensions(commercial.Extensions()))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(15 * time.Second):
				t.Error("Start() did not return after context cancellation")
			}
		})
	}
	t.Cleanup(stop)

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", cfg.MultiHost.GRPCListen, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 10*time.Second, 100*time.Millisecond, "the agent listener must come up")
	return a, stop
}

func createEnrollmentToken(t *testing.T, a *App) map[string]any {
	t.Helper()
	rec := serve(a, http.MethodPost, "/api/v1/agents/enrollment-tokens", `{"ttl_hours":1}`, nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// writeKeypair writes a self-signed keypair for 127.0.0.1 and returns both paths and the fingerprint.
func writeKeypair(t *testing.T) (certFile, keyFile, fingerprint string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "provided"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	certFile = filepath.Join(dir, "tls.crt")
	keyFile = filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile, derFingerprint(der)
}

func derFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func pemFileFingerprint(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return derFingerprint(cert.Raw)
}

func servedFingerprint(t *testing.T, addr string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- reading the served leaf
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	return derFingerprint(conn.ConnectionState().PeerCertificates[0].Raw)
}

// US1 scenarios 1 to 3, FR-001, FR-002, FR-003: without a keypair the certificate is
// written next to the database, served, logged, reported by the token route and
// pinned in the templates; a restart on the same database keeps it.
func TestStart_SelfSignedGRPCCertificateLivesNextToTheDatabase(t *testing.T) {
	proEdition(t)
	cfg, logs, logger := storageEnv(t)
	dataDir := filepath.Dir(cfg.DBPath)

	a, stop := multihostServer(t, &cfg, logger)
	crt := filepath.Join(dataDir, "grpc-server.crt")
	key := filepath.Join(dataDir, "grpc-server.key")
	want := pemFileFingerprint(t, crt)
	info, err := os.Stat(key)
	require.NoError(t, err, "the key sits next to the database")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, want, servedFingerprint(t, cfg.MultiHost.GRPCListen), "the listener serves the certificate on disk")
	assert.Contains(t, logs.String(), "fingerprint_sha256="+want, "the startup log names the fingerprint")

	out := createEnrollmentToken(t, a)
	assert.Equal(t, want, out["grpc_server_fingerprint_sha256"])
	assert.Equal(t, true, out["grpc_server_self_signed"])
	templates, ok := out["install_templates"].(map[string]any)
	require.True(t, ok)
	require.Len(t, templates, 4)
	for name, tmpl := range templates {
		assert.Contains(t, tmpl, want, "template %q must carry the fingerprint", name)
		assert.NotContains(t, tmpl, "grpc-insecure-skip-tls-verify", "template %q must not disable verification", name)
	}
	stop()

	second, _ := multihostServer(t, &cfg, logger)
	assert.Equal(t, want, pemFileFingerprint(t, crt), "the certificate file survives a restart")
	assert.Equal(t, want, createEnrollmentToken(t, second)["grpc_server_fingerprint_sha256"], "a restart keeps the fingerprint")
}

// US1 scenario 7 through the whole server: a provided keypair is reported, nothing is generated,
// and the templates carry no pin.
func TestStart_ProvidedGRPCCertificateIsReportedNotReplaced(t *testing.T) {
	proEdition(t)
	cfg, logs, logger := storageEnv(t)
	certFile, keyFile, provided := writeKeypair(t)
	cfg.MultiHost.TLSCertFile = certFile
	cfg.MultiHost.TLSKeyFile = keyFile

	a, _ := multihostServer(t, &cfg, logger)
	assert.Equal(t, provided, servedFingerprint(t, cfg.MultiHost.GRPCListen))
	_, err := os.Stat(filepath.Join(filepath.Dir(cfg.DBPath), "grpc-server.crt"))
	assert.True(t, os.IsNotExist(err), "no certificate is generated when one is provided")
	assert.Contains(t, logs.String(), "fingerprint_sha256="+provided)

	out := createEnrollmentToken(t, a)
	assert.Equal(t, provided, out["grpc_server_fingerprint_sha256"])
	assert.Equal(t, false, out["grpc_server_self_signed"])
	templates, ok := out["install_templates"].(map[string]any)
	require.True(t, ok)
	for name, tmpl := range templates {
		assert.NotContains(t, strings.ToLower(tmpl.(string)), "fingerprint", "template %q relies on the trust chain", name)
	}
}

// Edge case "agent embarqué": it pins the served certificate instead of skipping verification.
func TestStart_EmbeddedAgentPinsTheServedCertificate(t *testing.T) {
	proEdition(t)
	cfg, logs, logger := storageEnv(t)
	cfg.MultiHost.EmbeddedAgent = true

	multihostServer(t, &cfg, logger)

	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "stream authenticated")
	}, 30*time.Second, 200*time.Millisecond, "the embedded agent must enroll and authenticate over TLS:\n%s", logs.String())

	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, "component=embedded-agent") || !strings.Contains(line, "level=WARN") {
			continue
		}
		lower := strings.ToLower(line)
		assert.False(t, strings.Contains(lower, "verif") || strings.Contains(lower, "insecure") || strings.Contains(lower, "skip"),
			"the embedded agent must not run with verification disabled: %s", line)
	}
}
