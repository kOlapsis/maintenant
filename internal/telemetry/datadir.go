// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DataDirEnv is the environment variable that moves the telemetry identity off the default path.
const DataDirEnv = "MAINTENANT_TELEMETRY_DATADIR"

// DefaultDataDir reports where the telemetry identity lives when the caller gives no explicit directory.
func DefaultDataDir() string {
	if dir := strings.TrimSpace(os.Getenv(DataDirEnv)); dir != "" {
		return dir
	}
	return defaultDataDir
}

// ensureDataDirWritable creates the data directory if missing and verifies
// it is actually writable by performing a probe write/remove cycle. Per
// spec FR-015 / FR-016 / research.md R-003 — detecting unwritability
// up-front lets the caller emit the deterministic single-WARN log line
// instead of failing later on the first snapshot.
func ensureDataDirWritable(path string) error {
	if path == "" {
		return fmt.Errorf("telemetry: datadir path is empty")
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("telemetry: datadir %q not writable: %w", path, err)
	}
	probe := filepath.Join(path, ".maintenant-write-probe")
	if err := os.WriteFile(probe, []byte{}, 0o600); err != nil {
		return fmt.Errorf("telemetry: datadir %q not writable: %w", path, err)
	}
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("telemetry: datadir %q write-probe cleanup failed: %w", path, err)
	}
	return nil
}
