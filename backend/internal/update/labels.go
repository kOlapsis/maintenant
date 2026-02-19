package update

import (
	"log/slog"
	"strings"
)

const updateLabelPrefix = "pulseboard.update."

// ParseUpdateLabels extracts update configuration from Docker container labels.
func ParseUpdateLabels(labels map[string]string, logger *slog.Logger) UpdateConfig {
	cfg := UpdateConfig{
		Enabled: true, // enabled by default
		AlertOn: "all",
	}

	for key, value := range labels {
		if !strings.HasPrefix(key, updateLabelPrefix) {
			continue
		}
		suffix := key[len(updateLabelPrefix):]
		value = strings.TrimSpace(value)

		switch suffix {
		case "enabled":
			switch strings.ToLower(value) {
			case "false", "0", "no":
				cfg.Enabled = false
			case "true", "1", "yes":
				cfg.Enabled = true
			default:
				logger.Warn("invalid pulseboard.update.enabled value", "value", value)
			}
		case "track":
			switch strings.ToLower(value) {
			case "major", "minor", "patch", "digest":
				cfg.Track = strings.ToLower(value)
			default:
				logger.Warn("invalid pulseboard.update.track value", "value", value)
			}
		case "pin":
			cfg.Pin = value
		case "ignore_major":
			switch strings.ToLower(value) {
			case "true", "1", "yes":
				cfg.IgnoreMajor = true
			case "false", "0", "no":
				cfg.IgnoreMajor = false
			}
		case "registry":
			cfg.Registry = value
		case "alert_on":
			switch strings.ToLower(value) {
			case "all", "critical", "none":
				cfg.AlertOn = strings.ToLower(value)
			default:
				logger.Warn("invalid pulseboard.update.alert_on value", "value", value)
			}
		case "digest_only":
			switch strings.ToLower(value) {
			case "true", "1", "yes":
				cfg.DigestOnly = true
			}
		}
	}

	return cfg
}
