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
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/resource"
)

func TestConfigFromEnv_Retention(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		cfg := ConfigFromEnv()
		assert.Equal(t, resource.DefaultSnapshotRetention, cfg.Retention.Snapshots)
		assert.Equal(t, time.Hour, cfg.Retention.Interval)
		assert.Equal(t, 1000, cfg.Retention.BatchSize)
	})

	t.Run("values are read", func(t *testing.T) {
		t.Setenv("MAINTENANT_RETENTION_SNAPSHOTS", "48h")
		t.Setenv("MAINTENANT_RETENTION_INTERVAL", "15m")
		t.Setenv("MAINTENANT_RETENTION_BATCH_SIZE", "5000")

		cfg := ConfigFromEnv()
		assert.Equal(t, 48*time.Hour, cfg.Retention.Snapshots)
		assert.Equal(t, 15*time.Minute, cfg.Retention.Interval)
		assert.Equal(t, 5000, cfg.Retention.BatchSize)
	})

	// Unparseable or negative values fall back rather than disabling the purge.
	t.Run("invalid values fall back to defaults", func(t *testing.T) {
		t.Setenv("MAINTENANT_RETENTION_SNAPSHOTS", "abc")
		t.Setenv("MAINTENANT_RETENTION_INTERVAL", "-5m")
		t.Setenv("MAINTENANT_RETENTION_BATCH_SIZE", "not-a-number")

		cfg := ConfigFromEnv()
		assert.Equal(t, resource.DefaultSnapshotRetention, cfg.Retention.Snapshots)
		assert.Equal(t, time.Hour, cfg.Retention.Interval)
		assert.Equal(t, 1000, cfg.Retention.BatchSize)
	})
}

// MCP used to fail open: MAINTENANT_MCP=true with no OAuth credentials mounted
// /mcp with no auth at all and said so in a single Info line. Since the reverse
// proxy is documented as letting /mcp through, an incomplete .env published
// containers, logs and alerts. These lock the refusal in place.
func TestConfigValidateHTTP_MCP(t *testing.T) {
	base := func() Config {
		return Config{MCP: MCPConfig{
			Enabled:      true,
			ClientID:     "id",
			ClientSecret: "secret",
		}}
	}

	t.Run("credentials present", func(t *testing.T) {
		assert.NoError(t, base().ValidateHTTP())
	})

	t.Run("MCP disabled needs nothing", func(t *testing.T) {
		assert.NoError(t, Config{}.ValidateHTTP())
	})

	t.Run("missing client id is refused", func(t *testing.T) {
		cfg := base()
		cfg.MCP.ClientID = ""
		assert.ErrorIs(t, cfg.ValidateHTTP(), ErrMCPUnauthenticated)
	})

	t.Run("missing client secret is refused", func(t *testing.T) {
		cfg := base()
		cfg.MCP.ClientSecret = ""
		assert.ErrorIs(t, cfg.ValidateHTTP(), ErrMCPUnauthenticated)
	})

	// The opt-out is what keeps a local, trusted-network setup possible.
	t.Run("explicit opt-out is honoured", func(t *testing.T) {
		cfg := base()
		cfg.MCP.ClientID = ""
		cfg.MCP.ClientSecret = ""
		cfg.MCP.AllowUnauthenticated = true
		assert.NoError(t, cfg.ValidateHTTP())
	})
}

func TestConfigFromEnv_MCPAllowUnauthenticated(t *testing.T) {
	t.Run("absent by default", func(t *testing.T) {
		assert.False(t, ConfigFromEnv().MCP.AllowUnauthenticated)
	})

	t.Run("parsed with the shared truthy set", func(t *testing.T) {
		t.Setenv("MAINTENANT_MCP_ALLOW_UNAUTHENTICATED", "yes")
		assert.True(t, ConfigFromEnv().MCP.AllowUnauthenticated)
	})
}
