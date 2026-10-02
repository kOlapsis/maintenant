// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// Container labels that override the global configuration of one container.
const (
	LabelAnomaly            = "maintenant.anomaly"
	LabelAnomalySensitivity = "maintenant.anomaly.sensitivity"
	LabelAnomalyMetrics     = "maintenant.anomaly.metrics"
)

var absoluteMADFloor = map[string]float64{
	model.MetricCPU:       1.0,       // percentage points
	model.MetricMemory:    8 << 20,   // bytes
	model.MetricNetworkIO: 64 << 10,  // bytes per second
	model.MetricDiskIO:    64 << 10,  // bytes per second
	model.MetricLoad:      0.1,       // load per core
	model.MetricSwap:      8 << 20,   // bytes
	model.MetricDiskSpace: 128 << 20, // bytes
}

// AbsoluteMADFloor returns the smallest spread a metric is credited with, in its own unit.
func AbsoluteMADFloor(metric string) float64 {
	if v, ok := absoluteMADFloor[metric]; ok {
		return v
	}
	return 1e-6
}

// RelativeMADFloor is the fraction of |median| below which the spread is never taken.
const RelativeMADFloor = 0.05

// Config is the resolved global anomaly detection configuration.
type Config struct {
	Enabled            bool
	BaselineWindowDays int
	RequiredDays       int
	RelearnDays        int
	MinSamples         int
	SpikePersistence   int
	DetectInterval     time.Duration
	BaselineInterval   time.Duration
	Severity           string
	DefaultSensitivity string
	DefaultMetrics     []string
	Location           *time.Location
	TZName             string
}

// DefaultConfig returns the documented defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		BaselineWindowDays: 28,
		RequiredDays:       14,
		RelearnDays:        3,
		MinSamples:         4,
		SpikePersistence:   3,
		DetectInterval:     60 * time.Second,
		BaselineInterval:   time.Hour,
		Severity:           "warning",
		DefaultSensitivity: model.SensitivityMedium,
		DefaultMetrics:     []string{model.MetricCPU, model.MetricMemory},
		Location:           time.UTC,
		TZName:             "UTC",
	}
}

// ConfigFromEnv reads MAINTENANT_ANOMALY_* and MAINTENANT_TZ, keeping the default of any unset or invalid value.
func ConfigFromEnv(logger *slog.Logger) Config {
	if logger == nil {
		logger = slog.Default()
	}
	c := DefaultConfig()

	c.Enabled = envBool("MAINTENANT_ANOMALY_ENABLED", c.Enabled)
	c.BaselineWindowDays = envPositiveInt("MAINTENANT_ANOMALY_BASELINE_WINDOW_DAYS", c.BaselineWindowDays, logger)
	c.RequiredDays = envPositiveInt("MAINTENANT_ANOMALY_REQUIRED_DAYS", c.RequiredDays, logger)
	c.RelearnDays = envPositiveInt("MAINTENANT_ANOMALY_RELEARN_DAYS", c.RelearnDays, logger)
	c.MinSamples = envPositiveInt("MAINTENANT_ANOMALY_MIN_SAMPLES", c.MinSamples, logger)
	c.SpikePersistence = envPositiveInt("MAINTENANT_ANOMALY_SPIKE_PERSISTENCE", c.SpikePersistence, logger)
	c.DetectInterval = envDuration("MAINTENANT_ANOMALY_DETECT_INTERVAL", c.DetectInterval, logger)
	c.BaselineInterval = envDuration("MAINTENANT_ANOMALY_BASELINE_INTERVAL", c.BaselineInterval, logger)

	if sev := strings.ToLower(strings.TrimSpace(os.Getenv("MAINTENANT_ANOMALY_SEVERITY"))); sev != "" {
		if isValidSeverity(sev) {
			c.Severity = sev
		} else {
			logger.Warn("anomaly: invalid MAINTENANT_ANOMALY_SEVERITY, using default", "value", sev, "default", c.Severity)
		}
	}

	if tz := strings.TrimSpace(os.Getenv("MAINTENANT_TZ")); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			c.Location = loc
			c.TZName = tz
		} else {
			logger.Warn("anomaly: invalid MAINTENANT_TZ, using UTC", "value", tz, "error", err)
		}
	}

	return c
}

// SeriesConfig is the effective configuration of one scope.
type SeriesConfig struct {
	Enabled     bool
	Sensitivity string
	Metrics     []string
}

// ResolveContainer layers a container's labels over the global container defaults.
func (c Config) ResolveContainer(labels map[string]string, logger *slog.Logger) SeriesConfig {
	if logger == nil {
		logger = slog.Default()
	}
	sc := SeriesConfig{
		Enabled:     c.Enabled,
		Sensitivity: c.DefaultSensitivity,
		Metrics:     append([]string(nil), c.DefaultMetrics...),
	}

	if raw, ok := labels[LabelAnomaly]; ok {
		if v, valid := parseBool(raw); valid {
			sc.Enabled = c.Enabled && v
		} else {
			logger.Warn("anomaly: invalid "+LabelAnomaly+" label, ignoring", "value", raw)
		}
	}

	if raw, ok := labels[LabelAnomalySensitivity]; ok {
		s := strings.ToLower(strings.TrimSpace(raw))
		if isValidSensitivity(s) {
			sc.Sensitivity = s
		} else {
			logger.Warn("anomaly: invalid "+LabelAnomalySensitivity+" label, using default", "value", raw, "default", sc.Sensitivity)
		}
	}

	if raw, ok := labels[LabelAnomalyMetrics]; ok {
		if metrics := parseMetrics(raw, model.ScopeTypeContainer, logger); len(metrics) > 0 {
			sc.Metrics = metrics
		}
	}

	return sc
}

// Thresholds are the modified-z limits of a sensitivity: above K is active, between KLow and K is passive.
type Thresholds struct {
	K    float64
	KLow float64
}

// ThresholdsFor maps a sensitivity preset to its thresholds.
func ThresholdsFor(sensitivity string) Thresholds {
	switch sensitivity {
	case model.SensitivityHigh:
		return Thresholds{K: 3.0, KLow: 2.0}
	case model.SensitivityLow:
		return Thresholds{K: 5.0, KLow: 3.0}
	default:
		return Thresholds{K: 3.5, KLow: 2.5}
	}
}

func isValidSensitivity(s string) bool {
	return s == model.SensitivityLow || s == model.SensitivityMedium || s == model.SensitivityHigh
}

func isValidSeverity(s string) bool {
	return s == "info" || s == "warning" || s == "critical"
}

// parseMetrics returns the valid metrics of a comma list, nil when none is valid.
func parseMetrics(raw, scopeType string, logger *slog.Logger) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		m := strings.ToLower(strings.TrimSpace(part))
		if m == "" {
			continue
		}
		if !IsMetricValid(scopeType, m) {
			logger.Warn("anomaly: invalid metric in config, skipping", "metric", m, "scope_type", scopeType)
			continue
		}
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

func parseBool(raw string) (value, valid bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, true
	case "0", "f", "false", "n", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func envBool(key string, fallback bool) bool {
	if v, ok := parseBool(os.Getenv(key)); ok {
		return v
	}
	return fallback
}

func envPositiveInt(key string, fallback int, logger *slog.Logger) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	logger.Warn("anomaly: invalid integer env, using default", "key", key, "value", raw, "default", fallback)
	return fallback
}

func envDuration(key string, fallback time.Duration, logger *slog.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	logger.Warn("anomaly: invalid duration env, using default", "key", key, "value", raw, "default", fallback.String())
	return fallback
}
