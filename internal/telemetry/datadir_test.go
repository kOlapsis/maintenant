// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureDataDirWritable_HappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := ensureDataDirWritable(dir); err != nil {
		t.Fatalf("unexpected error on writable dir: %v", err)
	}
	probe := filepath.Join(dir, ".maintenant-write-probe")
	if _, err := os.Stat(probe); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe file not cleaned up: stat = %v", err)
	}
}

func TestEnsureDataDirWritable_CreatesMissingParent(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "nested", "deep", "shm")
	if err := ensureDataDirWritable(target); err != nil {
		t.Fatalf("expected MkdirAll to succeed for nested path, got %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target directory not created: %v", err)
	}
}

func TestEnsureDataDirWritable_ReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses POSIX write-bit checks")
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "ro")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	if err := os.Chmod(target, 0o500); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o755) })

	err := ensureDataDirWritable(target)
	if err == nil {
		t.Fatalf("expected error on read-only dir, got nil")
	}
}

func TestEnsureDataDirWritable_EmptyPath(t *testing.T) {
	if err := ensureDataDirWritable(""); err == nil {
		t.Fatalf("expected error on empty path, got nil")
	}
}
