// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"io"
	"log/slog"
	"testing"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestResolveContainerDefaults(t *testing.T) {
	cfg := DefaultConfig()
	sc := cfg.ResolveContainer(nil, quietLogger())
	assert.True(t, sc.Enabled)
	assert.Equal(t, model.SensitivityMedium, sc.Sensitivity)
	assert.Equal(t, []string{model.MetricCPU, model.MetricMemory}, sc.Metrics)
}

func TestResolveContainerOverrides(t *testing.T) {
	cfg := DefaultConfig()
	sc := cfg.ResolveContainer(map[string]string{
		LabelAnomalySensitivity: "high",
		LabelAnomalyMetrics:     "cpu, memory, network_io",
	}, quietLogger())
	assert.Equal(t, model.SensitivityHigh, sc.Sensitivity)
	assert.Equal(t, []string{model.MetricCPU, model.MetricMemory, model.MetricNetworkIO}, sc.Metrics)
}

func TestResolveContainerOptOut(t *testing.T) {
	cfg := DefaultConfig()
	sc := cfg.ResolveContainer(map[string]string{LabelAnomaly: "false"}, quietLogger())
	assert.False(t, sc.Enabled, "label opt-out disables the series even when the feature is on")
}

func TestResolveContainerInvalidLabelsFallBack(t *testing.T) {
	cfg := DefaultConfig()
	sc := cfg.ResolveContainer(map[string]string{
		LabelAnomalySensitivity: "extreme",        // invalid -> default
		LabelAnomalyMetrics:     "cpu,bogus,load", // load invalid for containers, bogus unknown
		LabelAnomaly:            "maybe",          // invalid -> keep global enabled
	}, quietLogger())
	assert.Equal(t, model.SensitivityMedium, sc.Sensitivity, "invalid sensitivity falls back to default")
	assert.Equal(t, []string{model.MetricCPU}, sc.Metrics, "only valid container metrics are kept")
	assert.True(t, sc.Enabled, "invalid enable label is ignored, global default kept")
}

func TestResolveContainerAllInvalidMetricsKeepsDefault(t *testing.T) {
	cfg := DefaultConfig()
	sc := cfg.ResolveContainer(map[string]string{LabelAnomalyMetrics: "load,bogus"}, quietLogger())
	assert.Equal(t, []string{model.MetricCPU, model.MetricMemory}, sc.Metrics,
		"an all-invalid metrics label leaves the default untouched")
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MAINTENANT_ANOMALY_ENABLED", "false")
	t.Setenv("MAINTENANT_ANOMALY_BASELINE_WINDOW_DAYS", "21")
	t.Setenv("MAINTENANT_ANOMALY_REQUIRED_DAYS", "10")
	t.Setenv("MAINTENANT_ANOMALY_DETECT_INTERVAL", "30s")
	t.Setenv("MAINTENANT_ANOMALY_BASELINE_INTERVAL", "2h")
	t.Setenv("MAINTENANT_ANOMALY_SEVERITY", "critical")
	t.Setenv("MAINTENANT_TZ", "America/New_York")

	cfg := ConfigFromEnv(quietLogger())
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 21, cfg.BaselineWindowDays)
	assert.Equal(t, 10, cfg.RequiredDays)
	assert.Equal(t, 30*time.Second, cfg.DetectInterval)
	assert.Equal(t, 2*time.Hour, cfg.BaselineInterval)
	assert.Equal(t, "critical", cfg.Severity)
	assert.Equal(t, "America/New_York", cfg.TZName)
	assert.NotNil(t, cfg.Location)
}

func TestConfigFromEnvInvalidFallsBack(t *testing.T) {
	t.Setenv("MAINTENANT_ANOMALY_BASELINE_WINDOW_DAYS", "-3")
	t.Setenv("MAINTENANT_ANOMALY_DETECT_INTERVAL", "not-a-duration")
	t.Setenv("MAINTENANT_ANOMALY_SEVERITY", "loud")
	t.Setenv("MAINTENANT_TZ", "Mars/Phobos")

	cfg := ConfigFromEnv(quietLogger())
	def := DefaultConfig()
	assert.Equal(t, def.BaselineWindowDays, cfg.BaselineWindowDays)
	assert.Equal(t, def.DetectInterval, cfg.DetectInterval)
	assert.Equal(t, def.Severity, cfg.Severity)
	assert.Equal(t, "UTC", cfg.TZName, "invalid TZ falls back to UTC")
}

func TestThresholdsFor(t *testing.T) {
	high := ThresholdsFor(model.SensitivityHigh)
	med := ThresholdsFor(model.SensitivityMedium)
	low := ThresholdsFor(model.SensitivityLow)
	// Higher sensitivity => lower K (catches smaller deviations).
	assert.Less(t, high.K, med.K)
	assert.Less(t, med.K, low.K)
	// Passive band lower bound is always below the active threshold.
	for _, th := range []Thresholds{high, med, low} {
		assert.Less(t, th.KLow, th.K)
	}
	// Unknown sensitivity defaults to medium.
	assert.Equal(t, med, ThresholdsFor("nonsense"))
}
