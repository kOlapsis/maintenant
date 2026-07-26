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

// Package trust holds the root certificates used to validate the TLS endpoints
// and certificates we monitor.
//
// It exists because Go's own escape hatch is a trap for this use case:
// SSL_CERT_FILE *replaces* the system bundle rather than extending it, so
// pointing it at an internal root silently drops every public CA — and when the
// file cannot be read, crypto/x509 returns an empty pool with a nil error, so
// every check fails as "unknown authority" with nothing in the logs. Operators
// running an internal PKI need to add a root, not swap the whole store.
package trust

import (
	"crypto/x509"
	"fmt"
	"os"
	"sync/atomic"
)

// pool is written once at startup, before any check runs, and only read
// afterwards. atomic.Pointer keeps that contract explicit and race-free.
var pool atomic.Pointer[x509.CertPool]

// Load reads a PEM bundle and appends it to the system roots. Any failure is
// returned rather than swallowed: a misconfigured CA path must stop the process,
// not degrade every TLS check into a confusing "unknown authority".
func Load(path string) error {
	// An empty path clears any previously loaded bundle, so Load fully describes
	// the desired state rather than only ever adding to it.
	if path == "" {
		pool.Store(nil)
		return nil
	}

	pem, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CA bundle path, read once at startup.
	if err != nil {
		return fmt.Errorf("read CA bundle %s: %w", path, err)
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("load system CA pool: %w", err)
	}

	if !roots.AppendCertsFromPEM(pem) {
		return fmt.Errorf("CA bundle %s contains no valid PEM certificate", path)
	}

	pool.Store(roots)
	return nil
}

// Pool returns the roots to validate against, or nil when no extra CA was
// loaded. Nil is meaningful: both tls.Config.RootCAs and x509.VerifyOptions.Roots
// read it as "use the system store", so an install without a custom CA keeps
// exactly the behaviour it had before this package existed.
func Pool() *x509.CertPool {
	return pool.Load()
}
