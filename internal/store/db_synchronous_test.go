package store

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func openSynchronous(t *testing.T, level string) *DB {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := OpenWithOptions(filepath.Join(t.TempDir(), "test.db"), Options{Synchronous: level}, logger)
	if err != nil {
		t.Fatalf("open with synchronous %q: %v", level, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func pragmaSynchronous(t *testing.T, db *DB) int {
	t.Helper()
	var v int
	if err := db.db.QueryRow("PRAGMA synchronous").Scan(&v); err != nil {
		t.Fatalf("read PRAGMA synchronous: %v", err)
	}
	return v
}

func TestSynchronousDefaultsToNormal(t *testing.T) {
	if got := pragmaSynchronous(t, openSynchronous(t, "")); got != 1 {
		t.Fatalf("PRAGMA synchronous = %d, want 1 (NORMAL)", got)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := Open(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if got := pragmaSynchronous(t, db); got != 1 {
		t.Fatalf("Open: PRAGMA synchronous = %d, want 1 (NORMAL)", got)
	}
}

func TestSynchronousFullWhenAsked(t *testing.T) {
	if got := pragmaSynchronous(t, openSynchronous(t, "FULL")); got != 2 {
		t.Fatalf("PRAGMA synchronous = %d, want 2 (FULL)", got)
	}
	if got := pragmaSynchronous(t, openSynchronous(t, "full")); got != 2 {
		t.Fatalf("lowercase full: PRAGMA synchronous = %d, want 2 (FULL)", got)
	}
}

func TestSynchronousRejectsUnknownLevels(t *testing.T) {
	for _, level := range []string{"OFF", "off", "EXTRA", "0", "2", "NORMA"} {
		t.Run(level, func(t *testing.T) {
			if _, err := NormalizeSynchronous(level); err == nil {
				t.Fatalf("NormalizeSynchronous(%q) accepted an unknown level", level)
			} else {
				for _, want := range []string{"NORMAL", "FULL"} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("error %q does not list %s", err, want)
					}
				}
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			db, err := OpenWithOptions(filepath.Join(t.TempDir(), "test.db"), Options{Synchronous: level}, logger)
			if err == nil {
				_ = db.Close()
				t.Fatalf("OpenWithOptions accepted synchronous %q", level)
			}
		})
	}
}
