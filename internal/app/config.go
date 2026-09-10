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
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/ratelimit"
	"github.com/kolapsis/maintenant/internal/resource"
	"github.com/kolapsis/maintenant/internal/store"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// Server
	Addr    string
	BaseURL string

	// Database
	DBPath string
	// DatabaseURL is the operator-supplied PostgreSQL connection string.
	// Empty — the default and the only supported agent setup — means SQLite
	// on DBPath. It carries a password: never log or render it directly,
	// always through store.RedactDSN.
	DatabaseURL string
	Retention   RetentionConfig

	// License
	LicenseKey string

	// SMTP
	SMTP SMTPConfig

	// MCP
	MCP MCPConfig

	// HTTP
	CORSOrigins string
	MaxBodySize int64
	// TrustedProxies lists the CIDRs and addresses whose forwarded headers are
	// believed, comma-separated. Empty means no header is ever read.
	TrustedProxies string

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

	// ContainerDownAfter is how long a container must stay stopped before it
	// raises an alert. Zero disables the check.
	ContainerDownAfter time.Duration
	// ContainerDownAfterInvalid holds a rejected threshold verbatim, so a typo
	// stops startup instead of silently leaving the check off.
	ContainerDownAfterInvalid string

	// Telemetry
	DisableTelemetry bool

	// Multi-host agent mode (Pro only)
	Mode      string // "embedded" | "server" | "agent"
	MultiHost MultiHostConfig

	// Dev
	AllowPrivateWebhooks bool

	// Runtime / logging (set via CLI flags; runtime override propagated to env)
	LogLevel string

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
	ServerURL                string
	EnrollmentToken          string
	RuntimeOverride          string
	Label                    string
	InsecureSkipVerify       bool
	EmbeddedAgent            bool
	AgentSpoolMaxMemoryBytes int64
	AgentSpoolMaxDiskBytes   int64
	AgentSpoolMaxAgeSeconds  int64
	InvalidSpoolSettings     []InvalidSetting
}

// InvalidSetting is a configuration value that was rejected rather than
// replaced by its default.
type InvalidSetting struct {
	Name string
	Raw  string
}

func (m *MultiHostConfig) acceptSpoolSetting(env string) {
	kept := m.InvalidSpoolSettings[:0]
	for _, s := range m.InvalidSpoolSettings {
		if s.Name != env {
			kept = append(kept, s)
		}
	}
	m.InvalidSpoolSettings = kept
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

// ErrDatabaseURLInAgentMode refuses an external database in agent mode rather
// than ignoring the setting: the agent's local store is not negotiable
// (FR-003, FR-030), and a silently dropped connection string would read as
// accepted.
var ErrDatabaseURLInAgentMode = errors.New(
	"MAINTENANT_DATABASE_URL is set but --mode=agent")

// ValidateHTTP rejects a configuration that must not be served over HTTP. It is
// deliberately not called from New(): --mcp-stdio shares that path and never
// listens, so refusing there would break a local stdio client for no gain.
// ValidateStorage refuses a storage configuration the mode cannot honour.
// The agent stores its state in SQLite, always (FR-003, FR-030); server and
// embedded both run the same server plane and accept an external database.
func (c Config) ValidateStorage() error {
	if c.DatabaseURL == "" {
		return nil
	}
	if c.Mode == "agent" {
		return fmt.Errorf("%w: the agent always stores its state locally", ErrDatabaseURLInAgentMode)
	}
	if _, err := store.ParseDSN(c.DatabaseURL); err != nil {
		return err
	}
	return nil
}

// ErrTrustedProxies refuses a proxy list that does not parse.
var ErrTrustedProxies = errors.New(
	"MAINTENANT_TRUSTED_PROXIES is not a valid list: use comma-separated CIDRs or IP addresses such as 10.0.0.0/8,192.168.1.4")

// ParseTrustedProxies returns the prefixes whose forwarded headers are believed.
func (c Config) ParseTrustedProxies() ([]netip.Prefix, error) {
	prefixes, err := ratelimit.ParsePrefixes(c.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrTrustedProxies, err)
	}
	return prefixes, nil
}

// ValidateProxies refuses a trusted-proxy list that would silently be ignored.
func (c Config) ValidateProxies() error {
	_, err := c.ParseTrustedProxies()
	return err
}

// ErrContainerDownAfter refuses a container-down threshold that does not parse.
var ErrContainerDownAfter = errors.New(
	"MAINTENANT_CONTAINER_DOWN_AFTER is not a valid duration: use a Go duration such as 5m, 30s or 1h30m")

// ValidateAlerting refuses an alerting configuration that would leave a check
// silently off.
func (c Config) ValidateAlerting() error {
	if c.ContainerDownAfterInvalid != "" {
		return fmt.Errorf("%w (got %q)", ErrContainerDownAfter, c.ContainerDownAfterInvalid)
	}
	return nil
}

const (
	DefaultAgentSpoolMaxMemoryBytes int64 = 16777216
	DefaultAgentSpoolMaxDiskBytes   int64 = 134217728
	DefaultAgentSpoolMaxAgeSeconds  int64 = 86400
)

// ErrAgentSpoolSetting refuses a spool budget that does not parse.
var ErrAgentSpoolSetting = errors.New(
	"agent spool setting is not a valid whole number of bytes or seconds: use a non-negative integer, or 0 to disable the spool")

func parseAgentSpoolSetting(raw string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%w (got %q)", ErrAgentSpoolSetting, raw)
	}
	return n, nil
}

func envAgentSpoolSetting(key string, fallback int64, invalid *[]InvalidSetting) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := parseAgentSpoolSetting(raw)
	if err != nil {
		*invalid = append(*invalid, InvalidSetting{Name: key, Raw: raw})
		return fallback
	}
	return n
}

// ValidateAgentSpool refuses a spool budget that would silently be replaced by
// its default.
func (c Config) ValidateAgentSpool() error {
	var errs []error
	for _, s := range c.MultiHost.InvalidSpoolSettings {
		errs = append(errs, fmt.Errorf("%w (%s=%q)", ErrAgentSpoolSetting, s.Name, s.Raw))
	}
	return errors.Join(errs...)
}

func (c Config) ValidateHTTP() error {
	if c.MCP.Enabled && !c.MCP.AllowUnauthenticated &&
		(c.MCP.ClientID == "" || c.MCP.ClientSecret == "") {
		return ErrMCPUnauthenticated
	}
	return nil
}

// DefaultAddr is the HTTP listen address used when MAINTENANT_ADDR is unset.
const DefaultAddr = "127.0.0.1:8080"

// ConfigFromEnv reads configuration from environment variables.
func ConfigFromEnv() Config {
	addr := envOr("MAINTENANT_ADDR", DefaultAddr)
	cfg := Config{
		Addr:        addr,
		BaseURL:     envOr("MAINTENANT_BASE_URL", "http://"+addr),
		DBPath:      envOr("MAINTENANT_DB", "./maintenant.db"),
		DatabaseURL: os.Getenv("MAINTENANT_DATABASE_URL"),

		LicenseKey: os.Getenv("MAINTENANT_LICENSE_KEY"),

		SMTP: SMTPConfig{
			Host:     os.Getenv("MAINTENANT_SMTP_HOST"),
			Port:     envOr("MAINTENANT_SMTP_PORT", "587"),
			Username: os.Getenv("MAINTENANT_SMTP_USERNAME"),
			Password: os.Getenv("MAINTENANT_SMTP_PASSWORD"),
			From:     envOr("MAINTENANT_SMTP_FROM", "maintenant@localhost"),
		},

		MCP: MCPConfig{
			Enabled:              parseTruthy(os.Getenv("MAINTENANT_MCP")),
			ClientID:             os.Getenv("MAINTENANT_MCP_CLIENT_ID"),
			ClientSecret:         os.Getenv("MAINTENANT_MCP_CLIENT_SECRET"),
			AllowedRedirectURIs:  os.Getenv("MAINTENANT_MCP_ALLOWED_REDIRECT_URIS"),
			AllowUnauthenticated: parseTruthy(os.Getenv("MAINTENANT_MCP_ALLOW_UNAUTHENTICATED")),
		},

		CORSOrigins:    os.Getenv("MAINTENANT_CORS_ORIGINS"),
		TrustedProxies: os.Getenv("MAINTENANT_TRUSTED_PROXIES"),
		MaxBodySize:    int64OrDefault("MAINTENANT_MAX_BODY_SIZE", 1048576),
		CACertFile:     os.Getenv("MAINTENANT_CA_CERT"),

		OrgName:   envOr("MAINTENANT_ORGANISATION_NAME", "Maintenant"),
		StatusURL: os.Getenv("MAINTENANT_STATUS_URL"),

		K8sNamespaces: os.Getenv("MAINTENANT_K8S_NAMESPACES"),
		K8sExcludeNS:  os.Getenv("MAINTENANT_K8S_EXCLUDE_NAMESPACES"),

		LogLevel: envOr("MAINTENANT_LOG_LEVEL", "info"),
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

	cfg.ContainerDownAfter, cfg.ContainerDownAfterInvalid = envOptionalDuration("MAINTENANT_CONTAINER_DOWN_AFTER")

	cfg.DisableTelemetry = parseTruthy(os.Getenv("MAINTENANT_DISABLE_TELEMETRY"))
	cfg.AllowPrivateWebhooks = parseTruthy(os.Getenv("MAINTENANT_ALLOW_PRIVATE_WEBHOOKS"))

	cfg.Mode = envOr("MAINTENANT_MODE", "embedded")

	cfg.MultiHost = MultiHostConfig{
		GRPCPublicURL:              os.Getenv("MAINTENANT_GRPC_URL"),
		GRPCListen:                 envOr("MAINTENANT_GRPC_LISTEN", "127.0.0.1:8443"),
		AgentRateLimitPerSecond:    envIntOr("MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND", 1000),
		AgentStaleThresholdSeconds: envIntOr("MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS", 60),
		TLSCertFile:                os.Getenv("MAINTENANT_GRPC_TLS_CERT"),
		TLSKeyFile:                 os.Getenv("MAINTENANT_GRPC_TLS_KEY"),
		InsecureGRPC:               parseTruthy(os.Getenv("MAINTENANT_GRPC_TLS_INSECURE")),
		ServerURL:                  os.Getenv("MAINTENANT_SERVER"),
		EnrollmentToken:            os.Getenv("MAINTENANT_ENROLLMENT_TOKEN"),
		RuntimeOverride:            os.Getenv("MAINTENANT_RUNTIME"),
		Label:                      os.Getenv("MAINTENANT_LABEL"),
		InsecureSkipVerify:         parseTruthy(os.Getenv("MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY")),
		EmbeddedAgent:              parseTruthy(os.Getenv("MAINTENANT_EMBEDDED_AGENT")),
	}

	var invalidSpool []InvalidSetting
	cfg.MultiHost.AgentSpoolMaxMemoryBytes = envAgentSpoolSetting(
		"MAINTENANT_AGENT_SPOOL_MAX_MEMORY_BYTES", DefaultAgentSpoolMaxMemoryBytes, &invalidSpool)
	cfg.MultiHost.AgentSpoolMaxDiskBytes = envAgentSpoolSetting(
		"MAINTENANT_AGENT_SPOOL_MAX_DISK_BYTES", DefaultAgentSpoolMaxDiskBytes, &invalidSpool)
	cfg.MultiHost.AgentSpoolMaxAgeSeconds = envAgentSpoolSetting(
		"MAINTENANT_AGENT_SPOOL_MAX_AGE_SECONDS", DefaultAgentSpoolMaxAgeSeconds, &invalidSpool)
	cfg.MultiHost.InvalidSpoolSettings = invalidSpool

	return cfg
}

func int64OrDefault(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

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

// envOptionalDuration parses a duration whose absence is meaningful. It returns
// the rejected raw value rather than a fallback: for a threshold that switches
// a check on, falling back to "off" would read as accepted.
func envOptionalDuration(key string) (time.Duration, string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, ""
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, v
	}
	return d, ""
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
