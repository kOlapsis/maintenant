// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/commercial/multihost"
	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// Acceptance tests of 046-agent-grpc-hardening, US1 (C11): the self-signed
// certificate is generated once, kept in the data directory, and its SHA-256
// fingerprint is logged and exposed to the API.

type silentBroadcaster struct{}

func (silentBroadcaster) BroadcastEvent(string, any) {}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLogger() (*slog.Logger, *lockedBuffer) {
	out := &lockedBuffer{}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug})), out
}

func sha256Hex(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func certFromPEMFile(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block, "%s is not PEM", path)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

func fileState(t *testing.T, path string) (content []byte, modTime time.Time) {
	t.Helper()
	content, err := os.ReadFile(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	return content, info.ModTime()
}

// makeKeypair returns a self-signed PEM keypair whose validity is given.
func makeKeypair(t *testing.T, notBefore, notAfter time.Time) (certPEM, keyPEM []byte, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "provided"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	cert, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), cert
}

func freeAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())
	return addr
}

// servedLeaf dials addr over TLS without verification and returns the leaf the server presented.
func servedLeaf(t *testing.T, addr string) *x509.Certificate {
	t.Helper()
	var leaf *x509.Certificate
	require.Eventually(t, func() bool {
		conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- reading the leaf is the point
		if err != nil {
			return false
		}
		defer func() { _ = conn.Close() }()
		leaf = conn.ConnectionState().PeerCertificates[0]
		return true
	}, 5*time.Second, 50*time.Millisecond, "the gRPC listener must answer a TLS handshake")
	return leaf
}

var fingerprintLine = regexp.MustCompile(`fingerprint_sha256=([0-9a-f]{64})`)

// US1 scenario 1, FR-001, FR-002: first start writes the keypair (key 0600) and logs the fingerprint.
func TestPersistentSelfSigned_WritesTheKeypairOnceAndLogsTheFingerprint(t *testing.T) {
	dir := t.TempDir()
	logger, logs := captureLogger()

	cfg, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1", "agents.example.test"}, logger)
	require.NoError(t, err)
	require.Len(t, cfg.Certificates, 1)

	keyInfo, err := os.Stat(filepath.Join(dir, "grpc-server.key"))
	require.NoError(t, err, "the private key is written next to the database")
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm(), "the private key is readable by the server only")
	crtInfo, err := os.Stat(filepath.Join(dir, "grpc-server.crt"))
	require.NoError(t, err, "the certificate is written next to the database")
	assert.Equal(t, os.FileMode(0o644), crtInfo.Mode().Perm())

	onDisk := certFromPEMFile(t, filepath.Join(dir, "grpc-server.crt"))
	assert.Equal(t, onDisk.Raw, cfg.Certificates[0].Certificate[0], "the served certificate is the one on disk")
	_, err = tls.LoadX509KeyPair(filepath.Join(dir, "grpc-server.crt"), filepath.Join(dir, "grpc-server.key"))
	require.NoError(t, err, "the key on disk matches the certificate on disk")

	fp := sha256Hex(onDisk.Raw)
	assert.Contains(t, logs.String(), "fingerprint_sha256="+fp, "the startup log names the fingerprint")
	assert.Contains(t, logs.String(), "self_signed=true")

	assert.InDelta(t, 10*365*24*time.Hour, time.Until(onDisk.NotAfter), float64(48*time.Hour), "ten years of validity")
	assert.Contains(t, onDisk.DNSNames, "agents.example.test")
	ips := make([]string, 0, len(onDisk.IPAddresses))
	for _, ip := range onDisk.IPAddresses {
		ips = append(ips, ip.String())
	}
	assert.Contains(t, ips, "127.0.0.1")
}

// US1 scenario 2, SC-001: ten restarts on the same directory give the same fingerprint and rewrite nothing.
func TestPersistentSelfSigned_ReusesTheKeypairAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	logger, logs := captureLogger()

	first, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1"}, logger)
	require.NoError(t, err)
	want := sha256Hex(first.Certificates[0].Certificate[0])
	crtBytes, crtTime := fileState(t, filepath.Join(dir, "grpc-server.crt"))
	keyBytes, keyTime := fileState(t, filepath.Join(dir, "grpc-server.key"))

	for i := range 9 {
		again, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1"}, logger)
		require.NoError(t, err, "restart %d", i+2)
		assert.Equal(t, want, sha256Hex(again.Certificates[0].Certificate[0]), "restart %d must serve the same certificate", i+2)
	}

	gotCrt, gotCrtTime := fileState(t, filepath.Join(dir, "grpc-server.crt"))
	gotKey, gotKeyTime := fileState(t, filepath.Join(dir, "grpc-server.key"))
	assert.Equal(t, crtBytes, gotCrt, "the certificate file is not rewritten")
	assert.Equal(t, crtTime, gotCrtTime, "the certificate file is not touched")
	assert.Equal(t, keyBytes, gotKey, "the key file is not rewritten")
	assert.Equal(t, keyTime, gotKeyTime, "the key file is not touched")

	assert.Equal(t, 10, strings.Count(logs.String(), "fingerprint_sha256="+want), "every start logs the fingerprint")
	assert.NotContains(t, logs.String(), "level=WARN", "a reused certificate is nothing to warn about")
}

// Two data directories never share a certificate: a fingerprint cannot be hard-coded.
func TestPersistentSelfSigned_DifferentDirectoriesDifferentCertificates(t *testing.T) {
	logger, _ := captureLogger()
	a, err := multihost.PersistentSelfSigned(t.TempDir(), []string{"127.0.0.1"}, logger)
	require.NoError(t, err)
	b, err := multihost.PersistentSelfSigned(t.TempDir(), []string{"127.0.0.1"}, logger)
	require.NoError(t, err)
	assert.NotEqual(t, sha256Hex(a.Certificates[0].Certificate[0]), sha256Hex(b.Certificates[0].Certificate[0]))
}

// Edge case "fichiers illisibles ou invalides", FR-001: a corrupt key, an expired certificate or a
// missing half are regenerated, with a WARN that pinned agents must be updated.
func TestPersistentSelfSigned_RegeneratesUnreadableOrExpiredFiles(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"corrupt key": func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc-server.key"), []byte("not a key"), 0o600))
		},
		"corrupt certificate": func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc-server.crt"), []byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n"), 0o644))
		},
		"expired certificate": func(t *testing.T, dir string) {
			certPEM, keyPEM, _ := makeKeypair(t, time.Now().Add(-48*time.Hour), time.Now().Add(-time.Hour))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc-server.crt"), certPEM, 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "grpc-server.key"), keyPEM, 0o600))
		},
		"missing key": func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "grpc-server.key")))
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			logger, logs := captureLogger()
			first, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1"}, logger)
			require.NoError(t, err)
			before := sha256Hex(first.Certificates[0].Certificate[0])

			damage(t, dir)
			logs.buf.Reset()

			again, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1"}, logger)
			require.NoError(t, err, "a damaged keypair is replaced, not fatal")
			after := sha256Hex(again.Certificates[0].Certificate[0])
			assert.NotEqual(t, before, after, "a regenerated certificate has a new fingerprint")

			onDisk := certFromPEMFile(t, filepath.Join(dir, "grpc-server.crt"))
			assert.Equal(t, after, sha256Hex(onDisk.Raw), "the new certificate is persisted")
			_, err = tls.LoadX509KeyPair(filepath.Join(dir, "grpc-server.crt"), filepath.Join(dir, "grpc-server.key"))
			require.NoError(t, err, "the new keypair on disk is loadable")
			assert.True(t, onDisk.NotAfter.After(time.Now().Add(9*365*24*time.Hour)), "the replacement is a fresh ten-year certificate")

			out := logs.String()
			assert.Contains(t, out, "level=WARN", "regeneration is announced as a warning")
			assert.Contains(t, out, "fingerprint_sha256="+after, "the new fingerprint is logged")
			assert.Regexp(t, `(?i)pinn`, out, "the warning says the pinned agents must be updated")
		})
	}
}

// Edge case "répertoire de données en lecture seule": startup is refused and the message names the directory.
func TestPersistentSelfSigned_RefusesAnUnwritableDirectory(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	dir := filepath.Join(blocker, "data")
	logger, _ := captureLogger()

	cfg, err := multihost.PersistentSelfSigned(dir, []string{"127.0.0.1"}, logger)
	require.Error(t, err, "no certificate can be kept in a directory that cannot be written")
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), dir, "the error names the directory")
}

func multiHostFor(t *testing.T, logger *slog.Logger) extpoint.MultiHost {
	t.Helper()
	return multihost.NewMultiHost(extpoint.MultiHostDeps{
		AgentStore:  store.NewAgentStore(storetest.Open(t, logger)),
		Broadcaster: silentBroadcaster{},
		Logger:      logger,
	})
}

func serveMultiHost(t *testing.T, mh extpoint.MultiHost, cfg extpoint.GRPCConfig) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, mh.Serve(ctx, cfg))
}

// FR-001, FR-002, FR-003 through the factory: Serve without a keypair persists the
// certificate in DataDir, serves it, logs it and exposes it to the API; a second
// server on the same DataDir serves the same certificate.
func TestServe_SelfSignedCertificateIsPersistedAndExposed(t *testing.T) {
	dir := t.TempDir()
	logger, logs := captureLogger()
	mh := multiHostFor(t, logger)
	addr := freeAddr(t)

	serveMultiHost(t, mh, extpoint.GRPCConfig{Listen: addr, DataDir: dir})

	onDisk := certFromPEMFile(t, filepath.Join(dir, "grpc-server.crt"))
	want := sha256Hex(onDisk.Raw)
	keyInfo, err := os.Stat(filepath.Join(dir, "grpc-server.key"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())

	fp, selfSigned := mh.Sessions.GRPCServerCertificate()
	assert.Equal(t, want, fp, "the API sees the fingerprint of the certificate on disk")
	assert.True(t, selfSigned)
	assert.Equal(t, want, sha256Hex(servedLeaf(t, addr).Raw), "the listener serves the certificate on disk")

	m := fingerprintLine.FindStringSubmatch(logs.String())
	require.NotNil(t, m, "the startup log names the fingerprint")
	assert.Equal(t, want, m[1])
	assert.Contains(t, logs.String(), "self_signed=true")

	second := multiHostFor(t, logger)
	addr2 := freeAddr(t)
	serveMultiHost(t, second, extpoint.GRPCConfig{Listen: addr2, DataDir: dir})
	fp2, _ := second.Sessions.GRPCServerCertificate()
	assert.Equal(t, want, fp2, "a restart on the same data directory keeps the fingerprint")
	assert.Equal(t, want, sha256Hex(servedLeaf(t, addr2).Raw))
}

// US1 scenario 7: a provided keypair is served and reported as not self-signed; nothing is generated.
func TestServe_ProvidedCertificateIsExposedNotGenerated(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, provided := makeKeypair(t, time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	certFile := filepath.Join(t.TempDir(), "tls.crt")
	keyFile := filepath.Join(t.TempDir(), "tls.key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o644))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	logger, logs := captureLogger()
	mh := multiHostFor(t, logger)
	addr := freeAddr(t)
	serveMultiHost(t, mh, extpoint.GRPCConfig{Listen: addr, DataDir: dir, TLSCertFile: certFile, TLSKeyFile: keyFile})

	want := sha256Hex(provided.Raw)
	fp, selfSigned := mh.Sessions.GRPCServerCertificate()
	assert.Equal(t, want, fp, "the fingerprint is the provided certificate's")
	assert.False(t, selfSigned)
	assert.Equal(t, want, sha256Hex(servedLeaf(t, addr).Raw))

	_, err := os.Stat(filepath.Join(dir, "grpc-server.crt"))
	assert.True(t, os.IsNotExist(err), "no certificate is generated when one is provided")
	_, err = os.Stat(filepath.Join(dir, "grpc-server.key"))
	assert.True(t, os.IsNotExist(err), "no key is generated when one is provided")

	assert.Contains(t, logs.String(), "fingerprint_sha256="+want)
	assert.Contains(t, logs.String(), "self_signed=false")
}

// Edge case "MAINTENANT_GRPC_TLS_INSECURE=true": plaintext has no fingerprint at all.
func TestServe_PlaintextListenerHasNoFingerprint(t *testing.T) {
	dir := t.TempDir()
	logger, _ := captureLogger()
	mh := multiHostFor(t, logger)
	serveMultiHost(t, mh, extpoint.GRPCConfig{Listen: freeAddr(t), DataDir: dir, Insecure: true})

	fp, selfSigned := mh.Sessions.GRPCServerCertificate()
	assert.Equal(t, "", fp)
	assert.False(t, selfSigned)
	_, err := os.Stat(filepath.Join(dir, "grpc-server.crt"))
	assert.True(t, os.IsNotExist(err), "h2c generates nothing")
}
