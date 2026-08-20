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
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/store"
)

const cliSentinelPassword = "s3cr3t-Sentinel-cli"

// TestLogStorageStartupError_DistinctFamilies pins FR-018 and SC-005: every
// startup failure an operator can act on produces its own message, and the
// families are distinguishable from one another. A single "database error"
// would turn an adoption into a support ticket.
func TestLogStorageStartupError_DistinctFamilies(t *testing.T) {
	dsn := "postgres://app:" + cliSentinelPassword + "@db.internal:5432/maintenant"

	families := map[string]error{
		"agent mode":         fmt.Errorf("wrapped: %w", app.ErrDatabaseURLInAgentMode),
		"invalid dsn":        fmt.Errorf("open database: %w", store.ErrInvalidDSN),
		"unreachable":        fmt.Errorf("open database: %w", store.ErrUnreachable),
		"credentials":        fmt.Errorf("open database: %w", store.ErrAuthRefused),
		"version":            fmt.Errorf("open database: %w", store.ErrUnsupportedVersion),
		"schema from future": fmt.Errorf("run migrations: %w", store.ErrSchemaNewer),
	}

	messages := map[string]string{}
	for name, err := range families {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		require.True(t, logStorageStartupError(logger, err, dsn), "%s must be classified", name)

		out := buf.String()
		assert.Contains(t, out, "fix=", "%s must say what to correct", name)
		assert.NotContains(t, out, cliSentinelPassword, "%s must not leak the password (FR-021)", name)
		messages[name] = out
	}

	// Every family reads differently from every other one.
	for a, outA := range messages {
		for b, outB := range messages {
			if a >= b {
				continue
			}
			assert.NotEqual(t, firstLine(outA), firstLine(outB),
				"%q and %q must not read the same", a, b)
		}
	}

	// The version message names the minimum required.
	assert.Contains(t, messages["version"], "14")
}

// TestLogStorageStartupError_PassesThroughOtherErrors keeps the classifier
// honest: an error that is not a storage startup failure is left to the
// caller's generic handler rather than mislabelled.
func TestLogStorageStartupError_PassesThroughOtherErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	handled := logStorageStartupError(logger, errors.New("detect container runtime: no docker socket"), "")
	assert.False(t, handled)
	assert.Empty(t, buf.String())
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
