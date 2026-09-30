// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"log/slog"
	"regexp"
	"strings"
)

const updateLabelPrefix = "maintenant.update."

// ParseUpdateLabels extracts update configuration from Docker container labels.
func ParseUpdateLabels(labels map[string]string, logger *slog.Logger) UpdateConfig {
	cfg := UpdateConfig{
		Enabled: true, // enabled by default
		AlertOn: AlertOnAll,
	}

	for key, value := range labels {
		if !strings.HasPrefix(key, updateLabelPrefix) {
			continue
		}
		suffix := key[len(updateLabelPrefix):]
		value = strings.TrimSpace(value)

		switch suffix {
		case "enabled":
			if b, ok := parseLabelBool(value); ok {
				cfg.Enabled = b
			} else {
				logger.Warn("invalid maintenant.update.enabled value", "value", value)
			}
		case "track":
			switch strings.ToLower(value) {
			case TrackMajor, TrackMinor, TrackPatch, TrackDigest:
				cfg.Track = strings.ToLower(value)
			default:
				logger.Warn("invalid maintenant.update.track value", "value", value)
			}
		case "pin":
			cfg.Pin = value
		case "ignore_major":
			if b, ok := parseLabelBool(value); ok {
				cfg.IgnoreMajor = b
			} else {
				logger.Warn("invalid maintenant.update.ignore_major value", "value", value)
			}
		case "registry":
			cfg.Registry = value
		case "alert_on":
			switch strings.ToLower(value) {
			case AlertOnAll, AlertOnCritical, AlertOnNone:
				cfg.AlertOn = strings.ToLower(value)
			default:
				logger.Warn("invalid maintenant.update.alert_on value", "value", value)
			}
		case "digest_only":
			if b, ok := parseLabelBool(value); ok {
				cfg.DigestOnly = b
			} else {
				logger.Warn("invalid maintenant.update.digest_only value", "value", value)
			}
		case "tag-include":
			if value == "" {
				continue
			}
			re, err := regexp.Compile(value)
			if err != nil {
				logger.Warn("invalid maintenant.update.tag-include regex, label ignored",
					"pattern", value, "error", err)
				continue
			}
			cfg.TagInclude = re
		case "tag-exclude":
			if value == "" {
				continue
			}
			re, err := regexp.Compile(value)
			if err != nil {
				logger.Warn("invalid maintenant.update.tag-exclude regex, label ignored",
					"pattern", value, "error", err)
				continue
			}
			cfg.TagExclude = re
		}
	}

	return cfg
}

func parseLabelBool(value string) (bool, bool) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	default:
		return false, false
	}
}
