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

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kolapsis/maintenant/internal/telemetry"
)

// StateRoot is where every mutable file the server writes lives, resolved once at startup.
type StateRoot struct {
	Dir string // MAINTENANT_STATE_DIR, empty for the historical paths

	DBPath           string // SQLite file (plus -wal and -shm)
	LicenseDir       string // .maintenant-license, .maintenant-update-window
	EmbeddedAgentDir string // identity.json, health
	TelemetryDir     string // telemetry identity
	TLSDir           string // gRPC certificate pair, empty without Dir
	TmpDir           string // SQLITE_TMPDIR, empty without Dir
}

// ErrStateDirNotAbsolute is returned when MAINTENANT_STATE_DIR is relative.
var ErrStateDirNotAbsolute = errors.New("MAINTENANT_STATE_DIR must be an absolute path")

// ResolveStateRoot resolves the state root from cfg, failing when a state directory is set but missing or not writable.
func ResolveStateRoot(cfg Config) (StateRoot, error) {
	dir := cfg.StateDir
	if dir == "" {
		return stateRootWithoutDir(cfg), nil
	}
	if !filepath.IsAbs(dir) {
		return StateRoot{}, fmt.Errorf("%w, got %q", ErrStateDirNotAbsolute, dir)
	}
	dir = filepath.Clean(dir)
	if err := checkStateDirUsable(dir); err != nil {
		return StateRoot{}, err
	}

	root := StateRoot{
		Dir:              dir,
		DBPath:           filepath.Join(dir, "maintenant.db"),
		LicenseDir:       dir,
		EmbeddedAgentDir: filepath.Join(dir, "embedded-agent"),
		TelemetryDir:     filepath.Join(dir, "shm"),
		TLSDir:           filepath.Join(dir, "tls"),
		TmpDir:           filepath.Join(dir, "tmp"),
	}
	if filepath.IsAbs(cfg.DBPath) {
		root.DBPath = cfg.DBPath
	}
	return root, nil
}

func stateRootWithoutDir(cfg Config) StateRoot {
	base := filepath.Dir(cfg.DBPath)
	return StateRoot{
		DBPath:           cfg.DBPath,
		LicenseDir:       base,
		EmbeddedAgentDir: filepath.Join(base, "embedded-agent"),
		TelemetryDir:     telemetry.DefaultDataDir(),
	}
}

func checkStateDirUsable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("state directory %q (MAINTENANT_STATE_DIR) is unusable: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("state directory %q (MAINTENANT_STATE_DIR) is not a directory", dir)
	}
	probe, err := os.CreateTemp(dir, ".maintenant-write-probe-*")
	if err != nil {
		return fmt.Errorf("state directory %q (MAINTENANT_STATE_DIR) is not writable: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("state directory %q (MAINTENANT_STATE_DIR) write probe failed: %w", dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("state directory %q (MAINTENANT_STATE_DIR) write probe cleanup failed: %w", dir, err)
	}
	return nil
}

// prepareStateRoot creates the directories the state root owns and points SQLITE_TMPDIR at it.
func prepareStateRoot(root StateRoot) error {
	if root.TmpDir == "" {
		return nil
	}
	if err := os.MkdirAll(root.TmpDir, 0o700); err != nil {
		return fmt.Errorf("create state temporary directory %q: %w", root.TmpDir, err)
	}
	if err := os.Setenv("SQLITE_TMPDIR", root.TmpDir); err != nil {
		return fmt.Errorf("point SQLITE_TMPDIR at %q: %w", root.TmpDir, err)
	}
	return nil
}
