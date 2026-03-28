// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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
				logger.Warn("invalid maintenant.update.enabled value", "value", value)
			}
		case "track":
			switch strings.ToLower(value) {
			case "major", "minor", "patch", "digest":
				cfg.Track = strings.ToLower(value)
			default:
				logger.Warn("invalid maintenant.update.track value", "value", value)
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
				logger.Warn("invalid maintenant.update.alert_on value", "value", value)
			}
		case "digest_only":
			switch strings.ToLower(value) {
			case "true", "1", "yes":
				cfg.DigestOnly = true
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
