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

package agent

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteHealth_ThenCheckHealth(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	require.NoError(t, WriteHealth(dir, now))
	assert.NoError(t, CheckHealth(dir, HealthMaxAge, now.Add(30*time.Second)))
}

// A file left behind by an agent that stopped ticking must read as unhealthy,
// not as "no agent here": the container is up and doing nothing.
func TestCheckHealth_StaleStampIsUnhealthy(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	require.NoError(t, WriteHealth(dir, now))

	err := CheckHealth(dir, HealthMaxAge, now.Add(HealthMaxAge+time.Second))
	require.Error(t, err)
	assert.NotErrorIs(t, err, fs.ErrNotExist, "a stuck agent must not look like a missing one")
	assert.Contains(t, err.Error(), "ago")
}

// No liveness file means no agent runs here, which the healthcheck reads as
// "this is a server" — so the distinction has to survive as fs.ErrNotExist.
func TestCheckHealth_MissingFileReportsNotExist(t *testing.T) {
	err := CheckHealth(t.TempDir(), HealthMaxAge, time.Now())
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestCheckHealth_CorruptStampIsUnhealthy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(HealthPath(dir), []byte("not-a-stamp\n"), 0o644))

	err := CheckHealth(dir, HealthMaxAge, time.Now())
	require.Error(t, err)
	assert.NotErrorIs(t, err, fs.ErrNotExist)
}

// The reporter must publish liveness immediately: an agent is healthy as soon as
// it starts streaming, not one tick later.
func TestStartHealthReporter_WritesImmediatelyThenRefreshes(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	// Registered after t.TempDir, so cleanups run it first: the reporter is
	// stopped before the directory is removed. Without it the goroutine can
	// recreate the liveness file mid-RemoveAll and fail the cleanup.
	stopped := StartHealthReporter(t.Context(), dir, 10*time.Millisecond, logger)
	t.Cleanup(func() { <-stopped })
	require.NoError(t, CheckHealth(dir, HealthMaxAge, time.Now()),
		"liveness must be published before the first tick")

	first, err := os.Stat(HealthPath(dir))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		again, err := os.Stat(HealthPath(dir))
		return err == nil && again.ModTime().After(first.ModTime())
	}, time.Second, 10*time.Millisecond, "the reporter must keep refreshing the file")
}
