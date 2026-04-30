// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
)

// setupTestLogger returns a slog logger writing JSON records to the
// returned buffer. Tests use it to assert the exact key set / level of
// the single startup log line (FR-017).
func setupTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

// newTestConfig returns a Config whose datadir is t.TempDir() and whose
// endpoint is empty (callers override it). Useful for tests that don't
// want to touch the host filesystem.
func newTestConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Endpoint:    "http://127.0.0.1:0", // overridden per-test
		DataDir:     t.TempDir(),
		AppVersion:  "test",
		Environment: "test",
	}
}

// fakeCounter is a hand-rolled implementation of the counter interface
// used by provider tests.
type fakeCounter struct {
	value int
	err   error
	panic any
}

func (f fakeCounter) CountConfigured(_ context.Context) (int, error) {
	if f.panic != nil {
		panic(f.panic)
	}
	return f.value, f.err
}

// staticEdition implements editionSource for tests.
type staticEdition struct {
	value extension.Edition
	panic any
}

func (s staticEdition) Current() extension.Edition {
	if s.panic != nil {
		panic(s.panic)
	}
	return s.value
}

// findLogRecord scans the JSON-handler buffer for the first record whose
// "msg" matches the given prefix. Returns the parsed map and the record
// level string ("INFO" / "WARN").
func findLogRecord(t *testing.T, buf *bytes.Buffer, msgPrefix string) (map[string]any, string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		if msg, _ := rec["msg"].(string); strings.HasPrefix(msg, msgPrefix) {
			level, _ := rec["level"].(string)
			return rec, level
		}
	}
	t.Fatalf("no log record with msg prefix %q in buffer:\n%s", msgPrefix, buf.String())
	return nil, ""
}

// readOnlyDataDir returns a directory and chmods it 0o500 so writes fail.
// On Windows or as root, it returns "" and the caller should skip.
func readOnlyDataDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return ""
	}
	if os.Getuid() == 0 {
		return ""
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "ro")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(target, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o755) })
	return target
}
