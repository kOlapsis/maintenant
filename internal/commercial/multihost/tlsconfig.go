// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	selfSignedCertFile = "grpc-server.crt"
	selfSignedKeyFile  = "grpc-server.key"
	selfSignedValidity = 10 * 365 * 24 * time.Hour
)

// LoadOrGenerateTLS returns the agent gRPC server TLS config and whether its certificate is self-signed.
//
// A configured keypair is loaded from disk. Otherwise the self-signed
// certificate kept in dataDir is used (see PersistentSelfSigned); with no
// dataDir an in-memory certificate is generated and a warning is logged.
// hosts seeds the self-signed certificate SAN; empty defaults to 127.0.0.1 + localhost.
func LoadOrGenerateTLS(certFile, keyFile string, hosts []string, dataDir string, logger *slog.Logger) (*tls.Config, bool, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, false, fmt.Errorf("load agentserver TLS keypair: %w", err)
		}
		logger.Info("agent gRPC TLS certificate", "fingerprint_sha256", fingerprintDER(cert.Certificate[0]), "self_signed", false)
		return serverTLSConfig(cert), false, nil
	}
	if dataDir != "" {
		cfg, err := PersistentSelfSigned(dataDir, hosts, logger)
		return cfg, true, err
	}

	logger.Warn("agentserver: no TLS keypair and no data directory, generating an in-memory self-signed certificate, DO NOT USE IN PRODUCTION",
		"hosts", hosts)
	cert, _, _, err := generateSelfSigned(hosts)
	if err != nil {
		return nil, false, err
	}
	logger.Info("agent gRPC TLS certificate", "fingerprint_sha256", fingerprintDER(cert.Certificate[0]), "self_signed", true)
	return serverTLSConfig(cert), true, nil
}

// Fingerprint returns the lowercase hex SHA-256 of a certificate's DER bytes.
func Fingerprint(cert *x509.Certificate) string { return fingerprintDER(cert.Raw) }

func fingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// PersistentSelfSigned loads the self-signed server certificate kept in dataDir, generating and saving it when it is missing, unreadable or expired.
func PersistentSelfSigned(dataDir string, hosts []string, logger *slog.Logger) (*tls.Config, error) {
	certPath := filepath.Join(dataDir, selfSignedCertFile)
	keyPath := filepath.Join(dataDir, selfSignedKeyFile)

	cert, loadErr := loadSelfSigned(certPath, keyPath)
	if loadErr != nil {
		_, certStatErr := os.Stat(certPath)
		_, keyStatErr := os.Stat(keyPath)
		existed := certStatErr == nil || keyStatErr == nil

		var err error
		cert, err = createSelfSigned(dataDir, certPath, keyPath, hosts)
		if err != nil {
			return nil, err
		}
		if existed {
			logger.Warn("agent gRPC TLS certificate regenerated, agents pinned to the previous fingerprint must be updated",
				"reason", loadErr.Error(), "fingerprint_sha256", fingerprintDER(cert.Certificate[0]))
		}
	}
	logger.Info("agent gRPC TLS certificate", "fingerprint_sha256", fingerprintDER(cert.Certificate[0]), "self_signed", true)
	return serverTLSConfig(cert), nil
}

func loadSelfSigned(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	if cert.Leaf == nil {
		return tls.Certificate{}, errors.New("certificate has no leaf")
	}
	if time.Now().After(cert.Leaf.NotAfter) {
		return tls.Certificate{}, fmt.Errorf("certificate expired on %s", cert.Leaf.NotAfter.Format(time.RFC3339))
	}
	return cert, nil
}

func createSelfSigned(dataDir, certPath, keyPath string, hosts []string) (tls.Certificate, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("agent gRPC certificate directory %s: %w", dataDir, err)
	}
	cert, certDER, keyDER, err := generateSelfSigned(hosts)
	if err != nil {
		return tls.Certificate{}, err
	}
	for _, p := range []string{certPath, keyPath} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return tls.Certificate{}, fmt.Errorf("agent gRPC certificate directory %s: %w", dataDir, err)
		}
	}
	if err := writeExclusive(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("agent gRPC certificate directory %s: %w", dataDir, err)
	}
	if err := writeExclusive(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		return tls.Certificate{}, fmt.Errorf("agent gRPC certificate directory %s: %w", dataDir, err)
	}
	return cert, nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- data directory + constant file name
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func serverTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
}

func generateSelfSigned(hosts []string) (cert tls.Certificate, certDER, keyDER []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("generate agentserver key: %w", err)
	}

	ips, dnsNames := splitHosts(hosts)
	if len(ips) == 0 && len(dnsNames) == 0 {
		ips = append(ips, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))
		dnsNames = append(dnsNames, "localhost")
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "maintenant-agentserver"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(selfSignedValidity),
		IPAddresses:  ips,
		DNSNames:     dnsNames,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err = x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("create agentserver certificate: %w", err)
	}
	keyDER, err = x509.MarshalECPrivateKey(key)
	if err != nil {
		return cert, nil, nil, fmt.Errorf("marshal agentserver key: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}, certDER, keyDER, nil
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
