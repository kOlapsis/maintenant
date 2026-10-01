// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package trusttest adds test certificates to the trust pool.
package trusttest

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolapsis/maintenant/internal/trust"
)

// Trust loads cert into the trust pool through MAINTENANT_CA_CERT's load path, until the test ends.
func Trust(t testing.TB, cert *x509.Certificate) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}
	if err := trust.Load(path); err != nil {
		t.Fatalf("load CA bundle: %v", err)
	}
	t.Cleanup(func() { _ = trust.Load("") })
}
