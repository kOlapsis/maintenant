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
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/resource"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// Server
	Addr    string
	BaseURL string

	// Database
	DBPath    string
	Retention RetentionConfig

	// License
	LicenseKey string

	// SMTP
	SMTP SMTPConfig

	// MCP
	MCP MCPConfig

	// HTTP
	CORSOrigins string
	MaxBodySize int64

	// CACertFile is a PEM bundle appended to the system roots, so endpoints and
	// certificates signed by an internal PKI validate without disabling checks.
	CACertFile string

	// Branding
	OrgName string

	// Status page
	StatusURL string // public URL of the status page (e.g. https://status.example.com)

	// Kubernetes
	K8sNamespaces string
	K8sExcludeNS  string

	// Security
	SecurityScoreThreshold int

	// Telemetry
	DisableTelemetry bool

	// Multi-host agent mode (Pro only)
	Mode      string // "embedded" | "server" | "agent"
	MultiHost MultiHostConfig

	// Dev
	AllowPrivateWebhooks bool

	// Build info (injected via ldflags)
	Version      string
	Commit       string
	BuildDate    string
	PublicKeyB64 string
}

// MultiHostConfig holds multi-server agent configuration (Pro only).
type MultiHostConfig struct {
	GRPCPublicURL              string
	GRPCListen                 string
	AgentRateLimitPerSecond    int
	AgentStaleThresholdSeconds int
	// TLS (for mode=server)
	TLSCertFile  string
	TLSKeyFile   string
	InsecureGRPC bool // h2c mode — use only behind a trusted reverse proxy
	// Agent flags (for mode=agent)
	ServerURL          string
	EnrollmentToken    string
	RuntimeOverride    string
	Label              string
	InsecureSkipVerify bool
	EmbeddedAgent      bool
}

// RetentionConfig holds the tunable part of the retention cleanup. Zero values
// mean "use the store defaults".
type RetentionConfig struct {
	// Snapshots is how long raw resource samples are kept. The 24h range is the
	// longest one reading them, so 24h is the floor; longer ranges are served
	// from the hourly rollup and are unaffected.
	Snapshots time.Duration
	Interval  time.Duration
	BatchSize int
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
	Enabled             bool
	ClientID            string
	ClientSecret        string
	AllowedRedirectURIs string
	// AllowUnauthenticated is the explicit opt-out for serving /mcp with no
	// OAuth at all; without it, MCP without credentials refuses to listen.
	AllowUnauthenticated bool
}

// ErrMCPUnauthenticated refuses to expose an MCP server that answers to anyone.
// docs/security.md has the reverse proxy let /mcp through unauthenticated, so
// missing OAuth credentials do not degrade the protection — they remove it.
var ErrMCPUnauthenticated = errors.New(
	"MAINTENANT_MCP=true but MAINTENANT_MCP_CLIENT_ID/MAINTENANT_MCP_CLIENT_SECRET are unset: " +
		"/mcp is documented as bypassing the reverse-proxy auth, so it would serve containers, logs " +
		"and alerts to anyone reaching it. Set both, or set MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true " +
		"to accept that on a trusted network")

// ValidateHTTP rejects a configuration that must not be served over HTTP. It is
// deliberately not called from New(): --mcp-stdio shares that path and never
// listens, so refusing there would break a local stdio client for no gain.
func (c Config) ValidateHTTP() error {
	if c.MCP.Enabled && !c.MCP.AllowUnauthenticated &&
		(c.MCP.ClientID == "" || c.MCP.ClientSecret == "") {
		return ErrMCPUnauthenticated
	}
	return nil
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
			Enabled:              os.Getenv("MAINTENANT_MCP") == "true",
			ClientID:             os.Getenv("MAINTENANT_MCP_CLIENT_ID"),
			ClientSecret:         os.Getenv("MAINTENANT_MCP_CLIENT_SECRET"),
			AllowedRedirectURIs:  os.Getenv("MAINTENANT_MCP_ALLOWED_REDIRECT_URIS"),
			AllowUnauthenticated: parseTruthy(os.Getenv("MAINTENANT_MCP_ALLOW_UNAUTHENTICATED")),
		},

		CORSOrigins: os.Getenv("MAINTENANT_CORS_ORIGINS"),
		MaxBodySize: 1048576,
		CACertFile:  os.Getenv("MAINTENANT_CA_CERT"),

		OrgName:   envOr("MAINTENANT_ORGANISATION_NAME", "Maintenant"),
		StatusURL: os.Getenv("MAINTENANT_STATUS_URL"),

		K8sNamespaces: os.Getenv("MAINTENANT_K8S_NAMESPACES"),
		K8sExcludeNS:  os.Getenv("MAINTENANT_K8S_EXCLUDE_NAMESPACES"),
	}

	if thresholdStr := os.Getenv("MAINTENANT_SECURITY_SCORE_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.Atoi(thresholdStr); err == nil && threshold > 0 {
			cfg.SecurityScoreThreshold = threshold
		}
	}

	cfg.Retention = RetentionConfig{
		Snapshots: envDurationOr("MAINTENANT_RETENTION_SNAPSHOTS", resource.DefaultSnapshotRetention),
		Interval:  envDurationOr("MAINTENANT_RETENTION_INTERVAL", time.Hour),
		BatchSize: envIntOr("MAINTENANT_RETENTION_BATCH_SIZE", 1000),
	}

	cfg.DisableTelemetry = parseTruthy(os.Getenv("MAINTENANT_DISABLE_TELEMETRY"))
	cfg.AllowPrivateWebhooks = parseTruthy(os.Getenv("MAINTENANT_ALLOW_PRIVATE_WEBHOOKS"))

	cfg.MultiHost = MultiHostConfig{
		GRPCPublicURL:              os.Getenv("MAINTENANT_GRPC_URL"),
		GRPCListen:                 envOr("MAINTENANT_GRPC_LISTEN", "127.0.0.1:8443"),
		AgentRateLimitPerSecond:    envIntOr("MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND", 1000),
		AgentStaleThresholdSeconds: envIntOr("MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS", 60),
		TLSCertFile:                os.Getenv("MAINTENANT_GRPC_TLS_CERT"),
		TLSKeyFile:                 os.Getenv("MAINTENANT_GRPC_TLS_KEY"),
		InsecureGRPC:               parseTruthy(os.Getenv("MAINTENANT_GRPC_TLS_INSECURE")),
	}

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

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
