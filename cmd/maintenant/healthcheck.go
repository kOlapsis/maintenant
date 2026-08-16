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

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/app"
)

const healthcheckTimeout = 4 * time.Second

// runHealthcheck backs the image's HEALTHCHECK, which has to serve both modes
// from a single command. An agent has no port to probe, so it reports liveness
// through a file in its data directory; a server answers on the address it was
// actually configured with, which a hardcoded localhost:8080 probe would miss.
// Returns the process exit code.
func runHealthcheck() int {
	dataDir := os.Getenv("MAINTENANT_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultAgentDataDir
	}

	err := agent.CheckHealth(dataDir, agent.HealthMaxAge, time.Now())
	switch {
	case err == nil:
		fmt.Println("agent alive")
		return 0
	case !errors.Is(err, fs.ErrNotExist):
		// The liveness file is there but stale or unreadable: this is an agent,
		// and it stopped ticking.
		fmt.Fprintln(os.Stderr, "agent unhealthy:", err)
		return 1
	}

	// No liveness file: server or embedded mode, whose health is served over HTTP.
	addr := os.Getenv("MAINTENANT_ADDR")
	if addr == "" {
		addr = app.DefaultAddr
	}
	if err := probeServerHealth(addr, healthcheckTimeout); err != nil {
		fmt.Fprintln(os.Stderr, "server unhealthy:", err)
		return 1
	}
	fmt.Println("server healthy")
	return 0
}

// probeServerHealth calls the health endpoint on the loopback interface. A
// wildcard listen address is reached through 127.0.0.1: the probe runs inside
// the container, so it never needs the published one.
func probeServerHealth(addr string, timeout time.Duration) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := "http://" + net.JoinHostPort(host, port) + "/api/v1/health"
	// #nosec G704 -- the target is our own listen address (MAINTENANT_ADDR), read by a local CLI check, never a caller-supplied URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// #nosec G704 -- same local listen address as above
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return nil
}
