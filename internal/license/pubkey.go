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

	return ed25519.PublicKey(raw), nil
}
