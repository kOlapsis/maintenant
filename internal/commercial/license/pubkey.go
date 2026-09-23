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

package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// publicKeyB64 is the base64-encoded Ed25519 public key for license signature
// verification. Injected at build time via -ldflags.
var publicKeyB64 string

func InitPublicKey(key string) {
	publicKeyB64 = key
}

// getPublicKey decodes the build-time public key and returns it.
// Returns an error if the key is missing or invalid.
func getPublicKey() (ed25519.PublicKey, error) {
	if publicKeyB64 == "" {
		return nil, fmt.Errorf("license public key not set (missing build-time injection)")
	}

	raw, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("invalid license public key encoding: %w", err)
	}

	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid license public key size: got %d, want %d", len(raw), ed25519.PublicKeySize)
	}

	return raw, nil
}
