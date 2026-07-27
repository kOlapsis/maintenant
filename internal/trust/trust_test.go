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

package trust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetPool clears the package state so each test starts from "no extra CA".
func resetPool(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { pool.Store(nil) })
	pool.Store(nil)
}

// writeCA generates a self-signed CA and writes it as PEM, returning its path.
func writeCA(t *testing.T, dir string) (string, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "maintenant-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	path := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return path, parsed
}

// An install with no custom CA must behave exactly as before this package:
// a nil pool tells crypto/tls and crypto/x509 to use the system store.
func TestPool_NilWhenUnconfigured(t *testing.T) {
	resetPool(t)

	require.NoError(t, Load(""))
	assert.Nil(t, Pool(), "an empty path must leave the system store untouched")
}

func TestLoad_AppendsToSystemRoots(t *testing.T) {
	resetPool(t)

	path, ca := writeCA(t, t.TempDir())
	require.NoError(t, Load(path))

	p := Pool()
	require.NotNil(t, p)

	// The custom root is trusted...
	_, err := ca.Verify(x509.VerifyOptions{Roots: p})
	assert.NoError(t, err, "the loaded CA must be trusted by the resulting pool")

	// ...and it did not replace the store, which is what SSL_CERT_FILE does.
	// Compared against a pool holding only our CA: they must differ, meaning the
	// system roots are still in there. (Subjects() is useless here — it returns
	// nothing for a system-derived pool.)
	onlyCustom := x509.NewCertPool()
	onlyCustom.AddCert(ca)
	assert.False(t, p.Equal(onlyCustom),
		"the pool must be the system store plus our CA, not our CA on its own")
}

// The whole point of this package: a misconfiguration is loud. SSL_CERT_FILE
// answers an unreadable file with an empty pool and a nil error.
func TestLoad_MissingFileIsAnError(t *testing.T) {
	resetPool(t)

	err := Load(filepath.Join(t.TempDir(), "absent.pem"))

	require.Error(t, err)
	assert.Nil(t, Pool(), "a failed load must not leave a half-built pool behind")
}

func TestLoad_UnreadableFileIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	resetPool(t)

	path := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(path, []byte("whatever"), 0o000))

	err := Load(path)

	require.Error(t, err, "an unreadable bundle is the exact case SSL_CERT_FILE swallows")
	assert.Nil(t, Pool())
}

func TestLoad_NonCertificateContentIsAnError(t *testing.T) {
	resetPool(t)

	path := filepath.Join(t.TempDir(), "notacert.pem")
	require.NoError(t, os.WriteFile(path, []byte("-----BEGIN NONSENSE-----\nzzz\n-----END NONSENSE-----\n"), 0o600))

	err := Load(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid PEM certificate")
	assert.Nil(t, Pool())
}
