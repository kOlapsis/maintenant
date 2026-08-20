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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentModeRefusesDatabaseURL is FR-003 and FR-030 proven on the real
// binary: an agent handed a connection string refuses it and exits, rather
// than ignoring it silently. The agent stores its state in SQLite, always,
// whatever the server it reports to is configured with.
func TestAgentModeRefusesDatabaseURL(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}

	bin := filepath.Join(t.TempDir(), "maintenant")
	build := exec.Command("go", "build", "-o", bin, ".")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build the binary: %s", out)

	const password = "s3cr3t-Sentinel-agent"
	cmd := exec.Command(bin, "--mode=agent",
		"--database-url=postgres://app:"+password+"@db.internal:5432/maintenant")
	cmd.Env = append(os.Environ(), "MAINTENANT_DATA_DIR="+t.TempDir())
	combined, err := cmd.CombinedOutput()

	require.Error(t, err, "the agent must refuse the setting, not start with it ignored")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())

	output := string(combined)
	assert.Contains(t, output, "agent", "the message names the offending mode")
	assert.NotContains(t, output, password, "the refusal must not echo the credential (FR-021)")
}

// TestAgentDoesNotDependOnTheStore keeps the agent's storage boundary
// structural rather than a matter of discipline: internal/agent cannot reach
// the server's store package at all, so no configuration could ever point it
// at an external database (FR-030).
func TestAgentDoesNotDependOnTheStore(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "github.com/kolapsis/maintenant/internal/agent")
	out, err := cmd.Output()
	require.NoError(t, err)

	for _, dep := range strings.Split(string(out), "\n") {
		assert.NotEqual(t, "github.com/kolapsis/maintenant/internal/store", strings.TrimSpace(dep),
			"the agent must not depend on the server store package")
	}
}
