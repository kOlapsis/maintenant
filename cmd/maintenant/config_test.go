// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
)

func TestWarnIgnoredSettings_NamesTheVariableTheValueAndTheDefault(t *testing.T) {
	t.Setenv("MAINTENANT_RETENTION_SNAPSHOTS", "7d")
	var out bytes.Buffer

	cfg, _ := loadConfig(nil)
	warnIgnoredSettings(cfg, slog.New(slog.NewJSONHandler(&out, nil)))

	assert.Equal(t, 1, strings.Count(out.String(), "\n"))
	assert.Contains(t, out.String(), `"level":"WARN"`)
	assert.Contains(t, out.String(), `"variable":"MAINTENANT_RETENTION_SNAPSHOTS","value":"7d","default":"48h0m0s"`)
}

func TestWarnMistypedBooleans_NamesEachVariableOnce(t *testing.T) {
	t.Setenv("MAINTENANT_DISABLE_TELEMETRY", "ture")
	t.Setenv("MAINTENANT_MCP", "treu")
	t.Setenv("MAINTENANT_PROXY_LABELS", "yes")
	var out bytes.Buffer

	cfg, _ := loadConfig(nil)
	warnIgnoredSettings(cfg, slog.New(slog.NewJSONHandler(&out, nil)))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, 1, strings.Count(out.String(), `"variable":"MAINTENANT_DISABLE_TELEMETRY","value":"ture"`))
	assert.Equal(t, 1, strings.Count(out.String(), `"variable":"MAINTENANT_MCP","value":"treu"`))
	assert.Contains(t, lines[0], `"level":"WARN"`)
}

func TestLoadConfig_FlagOverridesAMalformedEnvironment(t *testing.T) {
	t.Setenv("MAINTENANT_RETENTION_SNAPSHOTS", "7d")

	cfg, _ := loadConfig([]string{"--retentionSnapshots", "72h"})

	assert.Empty(t, cfg.InvalidSettings)
	assert.Equal(t, 72*time.Hour, cfg.Retention.Snapshots)
}

func TestLoadConfig_MalformedFlagFallsBackToTheDefault(t *testing.T) {
	t.Setenv("MAINTENANT_RETENTION_BATCH_SIZE", "")
	t.Setenv("MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS", "")

	cfg, _ := loadConfig([]string{"--retentionBatchSize", "0", "--agentStaleThresholdSeconds", "-1"})

	assert.Equal(t, 1000, cfg.Retention.BatchSize)
	assert.Equal(t, 60, cfg.MultiHost.AgentStaleThresholdSeconds)
	assert.ElementsMatch(t, []app.InvalidSetting{
		{Name: "MAINTENANT_RETENTION_BATCH_SIZE", Raw: "0", Default: "1000"},
		{Name: "MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS", Raw: "-1", Default: "60"},
	}, cfg.InvalidSettings)
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
