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
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// Server
	Addr    string
	BaseURL string

	// Database
	DBPath string

	// License
	LicenseKey string

	// SMTP
	SMTP SMTPConfig

	// MCP
	MCP MCPConfig

	// HTTP
	CORSOrigins string
	MaxBodySize int64

	// Branding
	OrgName string

	// Kubernetes
	K8sNamespaces  string
	K8sExcludeNS   string

	// Security
	SecurityScoreThreshold int

	// Telemetry
	DisableTelemetry bool

	// Dev
	AllowPrivateWebhooks bool

	// Build info (injected via ldflags)
	Version      string
	Commit       string
	BuildDate    string
	PublicKeyB64 string
}

// SMTPConfig holds SMTP mail server configuration.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// MCPConfig holds Model Context Protocol server configuration.
type MCPConfig struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
}

// ConfigFromEnv reads configuration from environment variables.
func ConfigFromEnv() Config {
	addr := envOr("MAINTENANT_ADDR", "127.0.0.1:8080")
	cfg := Config{
		Addr:    addr,
		BaseURL: envOr("MAINTENANT_BASE_URL", "http://"+addr),
		DBPath:  envOr("MAINTENANT_DB", "./maintenant.db"),

		LicenseKey: os.Getenv("MAINTENANT_LICENSE_KEY"),

		SMTP: SMTPConfig{
			Host:     os.Getenv("MAINTENANT_SMTP_HOST"),
			Port:     envOr("MAINTENANT_SMTP_PORT", "587"),
			Username: os.Getenv("MAINTENANT_SMTP_USERNAME"),
			Password: os.Getenv("MAINTENANT_SMTP_PASSWORD"),
			From:     envOr("MAINTENANT_SMTP_FROM", "maintenant@localhost"),
		},

		MCP: MCPConfig{
			Enabled:      os.Getenv("MAINTENANT_MCP") == "true",
			ClientID:     os.Getenv("MAINTENANT_MCP_CLIENT_ID"),
			ClientSecret: os.Getenv("MAINTENANT_MCP_CLIENT_SECRET"),
		},

		CORSOrigins: os.Getenv("MAINTENANT_CORS_ORIGINS"),
		MaxBodySize: 1048576,

		OrgName: envOr("MAINTENANT_ORGANISATION_NAME", "Maintenant"),

		K8sNamespaces: os.Getenv("MAINTENANT_K8S_NAMESPACES"),
		K8sExcludeNS:  os.Getenv("MAINTENANT_K8S_EXCLUDE_NAMESPACES"),
	}

	if thresholdStr := os.Getenv("MAINTENANT_SECURITY_SCORE_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.Atoi(thresholdStr); err == nil && threshold > 0 {
			cfg.SecurityScoreThreshold = threshold
		}
	}

	cfg.DisableTelemetry = parseTruthy(os.Getenv("MAINTENANT_DISABLE_TELEMETRY"))
	cfg.AllowPrivateWebhooks = parseTruthy(os.Getenv("MAINTENANT_ALLOW_PRIVATE_WEBHOOKS"))

	return cfg
}

// parseTruthy mirrors internal/telemetry/env.go semantics so the config
// layer does not depend on the telemetry package (avoids an import cycle
// at app wiring time). Truthy values: 1, t, true, y, yes, on
// (case-insensitive, whitespace-trimmed).
func parseTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
