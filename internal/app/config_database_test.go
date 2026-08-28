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
	"testing"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSentinelPassword = "s3cr3t-Sentinel"

func TestConfigFromEnv_ReadsDatabaseURL(t *testing.T) {
	// Unset: SQLite on MAINTENANT_DB, exactly as before (FR-001).
	cfg := ConfigFromEnv()
	assert.Empty(t, cfg.DatabaseURL)
	assert.Equal(t, "./maintenant.db", cfg.DBPath)

	t.Setenv("MAINTENANT_DATABASE_URL", "postgres://app@db.internal:5432/maintenant")
	cfg = ConfigFromEnv()
	assert.Equal(t, "postgres://app@db.internal:5432/maintenant", cfg.DatabaseURL)
	assert.Equal(t, "./maintenant.db", cfg.DBPath, "MAINTENANT_DB keeps its own meaning")
}

func TestValidateStorage(t *testing.T) {
	const dsn = "postgres://app:" + testSentinelPassword + "@db.internal:5432/maintenant"

	t.Run("no connection string is always valid", func(t *testing.T) {
		for _, mode := range []string{"", "embedded", "server", "agent"} {
			require.NoError(t, Config{Mode: mode}.ValidateStorage(), "mode %q", mode)
		}
	})

	t.Run("agent mode refuses it explicitly", func(t *testing.T) {
		err := Config{Mode: "agent", DatabaseURL: dsn}.ValidateStorage()
		require.ErrorIs(t, err, ErrDatabaseURLInAgentMode, "FR-003: refused, never silently ignored")
		assert.Contains(t, err.Error(), "agent", "the message names the offending mode")
		assert.NotContains(t, err.Error(), testSentinelPassword, "FR-021")
	})

	t.Run("server and embedded accept it", func(t *testing.T) {
		// Embedded runs the same server plane by another path, and is how a
		// multi-host fleet runs today: refusing it there would close the
		// feature to part of the target audience for a nominal reason.
		for _, mode := range []string{"server", "embedded", ""} {
			require.NoError(t, Config{Mode: mode, DatabaseURL: dsn}.ValidateStorage(), "mode %q", mode)
		}
	})

	t.Run("an unreadable connection string is refused", func(t *testing.T) {
		for _, raw := range []string{
			"not-a-dsn",
			"mysql://app@db/maintenant",
			"host=db user=app dbname=maintenant",
		} {
			err := Config{Mode: "server", DatabaseURL: raw}.ValidateStorage()
			require.ErrorIs(t, err, store.ErrInvalidDSN, raw)
		}
	})
}
