// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/trust"
)

// startTLSCapture serves a self-signed certificate for certName on a loopback
// listener and records the SNI presented by each connecting client.
func startTLSCapture(t *testing.T, certName string) (port int, lastSNI func() string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: certName},
		DNSNames:     []string{certName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	var mu sync.Mutex
	var sni string
	cfg := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			mu.Lock()
			sni = hello.ServerName
			mu.Unlock()
			return &cert, nil
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				_ = tls.Server(c, cfg).Handshake()
			}(conn)
		}
	}()

	return ln.Addr().(*net.TCPAddr).Port, func() string {
		mu.Lock()
		defer mu.Unlock()
		return sni
	}
}

func TestCheckCertificate_SNIPresentedAndValidated(t *testing.T) {
	const vhost = "sni.example.internal"
	port, lastSNI := startTLSCapture(t, vhost)

	result := CheckCertificate("127.0.0.1", port, vhost, 2*time.Second)

	require.Empty(t, result.Error)
	assert.Equal(t, vhost, lastSNI(), "server_name must be presented as SNI")
	assert.True(t, result.HostnameMatch, "certificate must be validated against server_name, not the dialled host")
	assert.Equal(t, vhost, result.SubjectCN)
}

func TestCheckCertificate_NoServerName_KeepsLegacyBehaviour(t *testing.T) {
	const vhost = "sni.example.internal"
	port, lastSNI := startTLSCapture(t, vhost)

	result := CheckCertificate("127.0.0.1", port, "", 2*time.Second)

	require.Empty(t, result.Error)
	// Go sends no SNI for IP literals — the pre-SNI behaviour for this dial.
	assert.Empty(t, lastSNI())
	assert.False(t, result.HostnameMatch, "cert for a DNS name must not match the dialled IP")
}

// trustedLeaf issues a certificate for name from a root added to the trust pool for the test.
func trustedLeaf(t *testing.T, name string) *x509.Certificate {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	bundle := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))
	require.NoError(t, trust.Load(bundle))
	t.Cleanup(func() { _ = trust.Load("") })

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	return leaf
}

// A certificate served under the wrong name is a hostname mismatch, not a broken chain.
func TestCheckCertificateFromPeerCerts_HostnameMismatchLeavesTheChainValid(t *testing.T) {
	leaf := trustedLeaf(t, "a.example.internal")

	mismatch := CheckCertificateFromPeerCerts([]*x509.Certificate{leaf}, "b.example.internal", nil)
	assert.False(t, mismatch.HostnameMatch)
	assert.True(t, mismatch.ChainValid, mismatch.ChainError)
	assert.Empty(t, mismatch.ChainError)

	match := CheckCertificateFromPeerCerts([]*x509.Certificate{leaf}, "a.example.internal", nil)
	assert.True(t, match.HostnameMatch)
	assert.True(t, match.ChainValid, match.ChainError)
}
