// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package license

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const cacheFileName = ".maintenant-license"

type cacheFile struct {
	SignedResponse
	ObservedAt time.Time `json:"observed_at"`
}

func cachePath(dataDir string) string {
	return filepath.Join(dataDir, cacheFileName)
}

// readCache returns the verified cached license and the latest time the instance observed, or nil if there is no valid cache.
func readCache(dataDir string, publicKey ed25519.PublicKey) (*LicensePayload, time.Time, error) {
	file, err := readCacheFile(dataDir)
	if err != nil || file == nil {
		return nil, time.Time{}, err
	}

	payload, err := verify(publicKey, file.SignedResponse)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("verifying cached license: %w", err)
	}

	return payload, file.ObservedAt, nil
}

func readCacheFile(dataDir string) (*cacheFile, error) {
	data, err := os.ReadFile(cachePath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading license cache: %w", err)
	}

	var file cacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing license cache: %w", err)
	}
	return &file, nil
}

// writeCache writes the signed response and the latest observed time to disk, replacing the file atomically.
func writeCache(dataDir string, resp *SignedResponse, observedAt time.Time) error {
	data, err := json.Marshal(cacheFile{SignedResponse: *resp, ObservedAt: observedAt.UTC()})
	if err != nil {
		return fmt.Errorf("marshaling license cache: %w", err)
	}

	tmp := cachePath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("writing license cache: %w", err)
	}
	if err := os.Rename(tmp, cachePath(dataDir)); err != nil {
		return fmt.Errorf("writing license cache: %w", err)
	}

	return nil
}

// touchCache records observedAt in the cache file, if there is one.
func touchCache(dataDir string, observedAt time.Time) error {
	file, err := readCacheFile(dataDir)
	if err != nil || file == nil {
		return err
	}
	return writeCache(dataDir, &file.SignedResponse, observedAt)
}

// deleteCache removes the cached license file.
func deleteCache(dataDir string) {
	_ = os.Remove(cachePath(dataDir))
}
