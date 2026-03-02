package license

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const cacheFileName = ".pulseboard-license"

func cachePath(dataDir string) string {
	return filepath.Join(dataDir, cacheFileName)
}

// readCache reads the cached license response from disk and verifies its
// signature. Returns nil if the cache is missing, corrupted, or tampered.
func readCache(dataDir string, publicKey ed25519.PublicKey) (*LicensePayload, error) {
	data, err := os.ReadFile(cachePath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading license cache: %w", err)
	}

	var resp SignedResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing license cache: %w", err)
	}

	payload, err := verify(publicKey, resp)
	if err != nil {
		return nil, fmt.Errorf("verifying cached license: %w", err)
	}

	return payload, nil
}

// writeCache writes the signed response to disk for offline verification.
func writeCache(dataDir string, resp *SignedResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling license cache: %w", err)
	}

	if err := os.WriteFile(cachePath(dataDir), data, 0600); err != nil {
		return fmt.Errorf("writing license cache: %w", err)
	}

	return nil
}

// deleteCache removes the cached license file.
func deleteCache(dataDir string) {
	os.Remove(cachePath(dataDir))
}
