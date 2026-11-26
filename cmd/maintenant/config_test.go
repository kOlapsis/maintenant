// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
)

func TestLoadConfig_FlagOverridesAMalformedEnvironment(t *testing.T) {
	t.Setenv("MAINTENANT_RETENTION_SNAPSHOTS", "7d")

	cfg, _ := loadConfig([]string{"--retentionSnapshots", "72h"})

	require.NoError(t, cfg.ValidateSettings())
	assert.Equal(t, 72*time.Hour, cfg.Retention.Snapshots)
}

func TestLoadConfig_MalformedFlagIsRefused(t *testing.T) {
	for flag, env := range map[string]string{
		"--retentionBatchSize": "MAINTENANT_RETENTION_BATCH_SIZE",
		"--updateInterval":     "MAINTENANT_UPDATE_INTERVAL",
	} {
		t.Setenv(env, "")

		cfg, _ := loadConfig([]string{flag, "0"})

		err := cfg.ValidateSettings()
		require.ErrorIs(t, err, app.ErrInvalidSetting, flag)
		assert.Contains(t, err.Error(), env)
	}
}

func TestLoadConfig_AddrFlagDrivesTheDefaultBaseURL(t *testing.T) {
	t.Setenv("MAINTENANT_ADDR", "")
	t.Setenv("MAINTENANT_BASE_URL", "")

	cfg, _ := loadConfig([]string{"--addr", "0.0.0.0:9090"})
	assert.Equal(t, "http://0.0.0.0:9090", cfg.BaseURL)

	t.Setenv("MAINTENANT_BASE_URL", "")
	cfg, _ = loadConfig([]string{"--addr", "0.0.0.0:9090", "--baseUrl", "https://now.example.com"})
	assert.Equal(t, "https://now.example.com", cfg.BaseURL)
}
