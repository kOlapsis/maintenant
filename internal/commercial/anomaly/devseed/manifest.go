// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package devseed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const manifestSuffix = ".devseed.json"

// manifest records, beside the database, the hours the seeder wrote so they can be withdrawn.
type manifest struct {
	Buckets map[string][]int64 `json:"buckets"`
}

func manifestPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), filepath.Base(dbPath)+manifestSuffix)
}

// loadManifest reads the sidecar; a missing or unreadable one means nothing to withdraw.
func loadManifest(path string) manifest {
	m := manifest{Buckets: map[string][]int64{}}
	raw, err := os.ReadFile(path) // #nosec G304 -- path derived from the operator's own database path
	if err != nil {
		return m
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Buckets == nil {
		return manifest{Buckets: map[string][]int64{}}
	}
	return m
}

func (m *manifest) add(containerID string, buckets []int64) {
	m.Buckets[containerID] = append(m.Buckets[containerID], buckets...)
}

func (m manifest) save(path string) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode seed manifest: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write seed manifest: %w", err)
	}
	return nil
}

func removeManifest(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove seed manifest: %w", err)
	}
	return nil
}
