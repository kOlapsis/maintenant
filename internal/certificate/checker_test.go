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

package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
