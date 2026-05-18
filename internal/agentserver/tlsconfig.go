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

package agentserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"time"
)

// LoadOrGenerateTLS returns a TLS config for the agent gRPC server.
//
// If both certFile and keyFile are non-empty, the keypair is loaded from
// disk. Otherwise a self-signed dev certificate is generated in-memory and
// a loud warning is logged — agents must then connect with
// --grpc-insecure-skip-tls-verify. Intended for local development only.
//
// hosts seeds the certificate SAN (Subject Alternative Names) so a strict
// client can still verify it. Empty/unknown hosts default to 127.0.0.1 + localhost.
func LoadOrGenerateTLS(certFile, keyFile string, hosts []string, logger *slog.Logger) (*tls.Config, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load agentserver TLS keypair: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}

	logger.Warn("agentserver: no TLS keypair configured, generating self-signed dev certificate — DO NOT USE IN PRODUCTION",
		"hosts", hosts)
	return generateSelfSignedTLS(hosts)
}

func generateSelfSignedTLS(hosts []string) (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate dev key: %w", err)
	}

	ips, dnsNames := splitHosts(hosts)
	if len(ips) == 0 && len(dnsNames) == 0 {
		ips = append(ips, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))
		dnsNames = append(dnsNames, "localhost")
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "maintenant-dev"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		IPAddresses:  ips,
		DNSNames:     dnsNames,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create dev cert: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{certDER},
			PrivateKey:  key,
		}},
		MinVersion: tls.VersionTLS12,
	}, nil
}

func splitHosts(hosts []string) (ips []net.IP, dnsNames []string) {
	seen := map[string]bool{}
	for _, h := range hosts {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}
	return ips, dnsNames
}
