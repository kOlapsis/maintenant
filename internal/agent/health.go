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
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// healthFile holds the epoch second of the agent's last loop tick. An agent
// speaks gRPC outbound only and serves no HTTP, so a container healthcheck has
// no port to probe: this file is what it reads instead.
const healthFile = "health"

// HealthInterval is how often a running agent refreshes its liveness file, and
// HealthMaxAge how stale that file may be before the agent counts as stuck.
// The gap between them absorbs a slow tick without flapping.
const (
	HealthInterval = 15 * time.Second
	HealthMaxAge   = 60 * time.Second
)

// HealthPath returns the liveness file path inside an agent data directory.
func HealthPath(dataDir string) string {
	return filepath.Join(dataDir, healthFile)
}

// WriteHealth records that the agent loop was alive at t. The file is replaced
// atomically so a healthcheck reading it concurrently never sees a partial stamp.
func WriteHealth(dataDir string, t time.Time) error {
	path := HealthPath(dataDir)
	tmp := path + ".tmp"
	stamp := strconv.FormatInt(t.Unix(), 10) + "\n"

	// 0644: the stamp is not a secret, and the healthcheck process may well run
	// under a different uid than the agent.
	if err := os.WriteFile(tmp, []byte(stamp), 0644); err != nil { // #nosec G306 -- liveness stamp, deliberately world-readable
		return fmt.Errorf("write liveness file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace liveness file: %w", err)
	}
	return nil
}

// StartHealthReporter refreshes the liveness file every interval until ctx is done.
//
// Reachability of the server is deliberately not part of the signal: an agent
// that cannot reach the server is already reported as disconnected there, and
// letting an orchestrator restart the agent over it would fix nothing while
// hiding the actual outage.
func StartHealthReporter(ctx context.Context, dataDir string, interval time.Duration, logger *slog.Logger) {
	report := func() {
		if err := WriteHealth(dataDir, time.Now()); err != nil {
			logger.Warn("agent: liveness file refresh failed", "err", err)
		}
	}
	report()

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				report()
			}
		}
	}()
}

// CheckHealth reports whether an agent refreshed its liveness file within maxAge.
// A missing file is returned as an fs.ErrNotExist-wrapped error so callers can
// tell "no agent runs here" from "the agent is stuck".
func CheckHealth(dataDir string, maxAge time.Duration, now time.Time) error {
	path := HealthPath(dataDir)
	data, err := os.ReadFile(path) // #nosec G304 -- path is dataDir + constant liveness filename, not user input
	if err != nil {
		return err
	}

	sec, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return fmt.Errorf("unreadable liveness stamp in %s: %w", path, err)
	}

	if age := now.Sub(time.Unix(sec, 0)); age > maxAge {
		return fmt.Errorf("agent last ticked %s ago, over the %s limit",
			age.Truncate(time.Second), maxAge)
	}
	return nil
}
