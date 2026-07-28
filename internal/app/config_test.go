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
)

func TestConfigFromEnv_Retention(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		cfg := ConfigFromEnv()
		assert.Equal(t, 7*24*time.Hour, cfg.Retention.Snapshots)
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
		assert.Equal(t, 7*24*time.Hour, cfg.Retention.Snapshots)
		assert.Equal(t, time.Hour, cfg.Retention.Interval)
		assert.Equal(t, 1000, cfg.Retention.BatchSize)
	})
}
