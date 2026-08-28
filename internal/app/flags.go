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
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// FlagType is the value type of a configuration flag.
type FlagType int

const (
	FlagTypeString FlagType = iota
	FlagTypeInt
	FlagTypeBool
	FlagTypeDuration
)

// FlagSpec describes one configuration option: its env var, CLI flag, type,
// default value, and how to apply it to a Config.
type FlagSpec struct {
	EnvName     string
	FlagName    string
	Type        FlagType
	Default     string
	Description string
	Sensitive   bool
	// NoEnv marks an action flag that has no environment equivalent
	// (--copy-store-to, --yes): it drives what the process does, it is not
	// part of the configuration an operator writes in maintenant.env.
	NoEnv   bool
	ApplyTo func(*Config, string) error
}

// FlagCategory groups related flags for --help display.
type FlagCategory struct {
	Name  string
	Specs []FlagSpec
}

// Registry is the single source of truth for all CLI/env configuration options.
var Registry []FlagSpec

// Categories organises Registry entries by section for --help output.
var Categories []FlagCategory

func init() {
	Registry = []FlagSpec{
		// Server
		{
			EnvName: "MAINTENANT_ADDR", FlagName: "addr",
			Type: FlagTypeString, Default: "127.0.0.1:8080",
			Description: "HTTP listen address (host:port)",
			ApplyTo:     func(c *Config, v string) error { c.Addr = v; return nil },
		},
		{
			EnvName: "MAINTENANT_BASE_URL", FlagName: "baseUrl",
			Type: FlagTypeString, Default: "",
			Description: "Public base URL for status pages and links",
			ApplyTo:     func(c *Config, v string) error { c.BaseURL = v; return nil },
		},
		{
			EnvName: "MAINTENANT_CORS_ORIGINS", FlagName: "corsOrigins",
			Type: FlagTypeString, Default: "",
			Description: "Comma-separated CORS origins (empty = same-origin)",
			ApplyTo:     func(c *Config, v string) error { c.CORSOrigins = v; return nil },
		},
		// Storage
		{
			EnvName: "MAINTENANT_DB", FlagName: "db",
			Type: FlagTypeString, Default: "./maintenant.db",
			Description: "SQLite database path",
			ApplyTo:     func(c *Config, v string) error { c.DBPath = v; return nil },
		},
		// Branding
		{
			EnvName: "MAINTENANT_ORGANISATION_NAME", FlagName: "organisationName",
			Type: FlagTypeString, Default: "Maintenant",
			Description: "Display name on public status page",
			ApplyTo:     func(c *Config, v string) error { c.OrgName = v; return nil },
		},
		// Runtime
		{
			EnvName: "MAINTENANT_RUNTIME", FlagName: "runtime",
			Type: FlagTypeString, Default: "",
			Description: "Force container runtime (docker|kubernetes; default: autodetect)",
			// Propagate to env so pbruntime.Detect() picks it up, and to the
			// agent config, which carries the same override over the wire.
			ApplyTo: func(c *Config, v string) error {
				c.MultiHost.RuntimeOverride = v
				return os.Setenv("MAINTENANT_RUNTIME", v)
			},
		},
		// Logging
		{
			EnvName: "MAINTENANT_LOG_LEVEL", FlagName: "logLevel",
			Type: FlagTypeString, Default: "info",
			Description: "Log level (debug|info|warn|error)",
			ApplyTo:     func(c *Config, v string) error { c.LogLevel = v; return nil },
		},
		// HTTP
		{
			EnvName: "MAINTENANT_MAX_BODY_SIZE", FlagName: "maxBodySize",
			Type: FlagTypeInt, Default: "1048576",
			Description: "Max request body size in bytes",
			ApplyTo: func(c *Config, v string) error {
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return fmt.Errorf("expected integer, got %q", v)
				}
				c.MaxBodySize = n
				return nil
			},
		},
		// Updates
		{
			EnvName: "MAINTENANT_UPDATE_INTERVAL", FlagName: "updateInterval",
			Type: FlagTypeDuration, Default: "24h",
			Description: "Update intelligence scan interval (Go duration)",
			// Propagate to env so internal/update/service.go picks it up
			ApplyTo: func(_ *Config, v string) error {
				if _, err := time.ParseDuration(v); err != nil {
					return fmt.Errorf("expected Go duration (e.g. 24h, 30m), got %q", v)
				}
				return os.Setenv("MAINTENANT_UPDATE_INTERVAL", v)
			},
		},
		// Security
		{
			EnvName: "MAINTENANT_SECURITY_SCORE_THRESHOLD", FlagName: "securityScoreThreshold",
			Type: FlagTypeInt, Default: "",
			Description: "Minimum security score threshold for alerts",
			ApplyTo: func(c *Config, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("expected integer, got %q", v)
				}
				c.SecurityScoreThreshold = n
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_DISABLE_TELEMETRY", FlagName: "disableTelemetry",
			Type: FlagTypeBool, Default: "false",
			Description: "Disable anonymous telemetry",
			ApplyTo: func(c *Config, v string) error {
				c.DisableTelemetry = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_ALLOW_PRIVATE_WEBHOOKS", FlagName: "allowPrivateWebhooks",
			Type: FlagTypeBool, Default: "false",
			Description: "Allow webhook targets on private/loopback addresses",
			ApplyTo: func(c *Config, v string) error {
				c.AllowPrivateWebhooks = parseTruthy(v)
				return nil
			},
		},
		// Pro
		{
			EnvName: "MAINTENANT_LICENSE_KEY", FlagName: "licenseKey",
			Type: FlagTypeString, Default: "", Sensitive: true,
			Description: "Pro license key (enables Pro features)",
			ApplyTo:     func(c *Config, v string) error { c.LicenseKey = v; return nil },
		},
		// SMTP
		{
			EnvName: "MAINTENANT_SMTP_HOST", FlagName: "smtpHost",
			Type: FlagTypeString, Default: "",
			Description: "SMTP server hostname",
			ApplyTo:     func(c *Config, v string) error { c.SMTP.Host = v; return nil },
		},
		{
			EnvName: "MAINTENANT_SMTP_PORT", FlagName: "smtpPort",
			Type: FlagTypeString, Default: "587",
			Description: "SMTP server port",
			ApplyTo:     func(c *Config, v string) error { c.SMTP.Port = v; return nil },
		},
		{
			EnvName: "MAINTENANT_SMTP_USERNAME", FlagName: "smtpUsername",
			Type: FlagTypeString, Default: "",
			Description: "SMTP username",
			ApplyTo:     func(c *Config, v string) error { c.SMTP.Username = v; return nil },
		},
		{
			EnvName: "MAINTENANT_SMTP_PASSWORD", FlagName: "smtpPassword",
			Type: FlagTypeString, Default: "", Sensitive: true,
			Description: "SMTP password",
			ApplyTo:     func(c *Config, v string) error { c.SMTP.Password = v; return nil },
		},
		{
			EnvName: "MAINTENANT_SMTP_FROM", FlagName: "smtpFrom",
			Type: FlagTypeString, Default: "maintenant@localhost",
			Description: "SMTP sender address",
			ApplyTo:     func(c *Config, v string) error { c.SMTP.From = v; return nil },
		},
		// MCP
		{
			EnvName: "MAINTENANT_MCP", FlagName: "mcp",
			Type: FlagTypeBool, Default: "false",
			Description: "Enable MCP server (Model Context Protocol)",
			ApplyTo: func(c *Config, v string) error {
				c.MCP.Enabled = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_MCP_CLIENT_ID", FlagName: "mcpClientId",
			Type: FlagTypeString, Default: "",
			Description: "MCP OAuth client ID",
			ApplyTo:     func(c *Config, v string) error { c.MCP.ClientID = v; return nil },
		},
		{
			EnvName: "MAINTENANT_MCP_CLIENT_SECRET", FlagName: "mcpClientSecret",
			Type: FlagTypeString, Default: "", Sensitive: true,
			Description: "MCP OAuth client secret",
			ApplyTo:     func(c *Config, v string) error { c.MCP.ClientSecret = v; return nil },
		},
		// Kubernetes
		{
			EnvName: "MAINTENANT_MCP_ALLOWED_REDIRECT_URIS", FlagName: "mcpAllowedRedirectUris",
			Type: FlagTypeString, Default: "",
			Description: "Comma-separated OAuth redirect URIs accepted by /mcp",
			ApplyTo:     func(c *Config, v string) error { c.MCP.AllowedRedirectURIs = v; return nil },
		},
		{
			EnvName: "MAINTENANT_MCP_ALLOW_UNAUTHENTICATED", FlagName: "mcpAllowUnauthenticated",
			Type: FlagTypeBool, Default: "false",
			Description: "Serve /mcp with no credentials (trusted network only)",
			ApplyTo: func(c *Config, v string) error {
				c.MCP.AllowUnauthenticated = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_K8S_NAMESPACES", FlagName: "k8sNamespaces",
			Type: FlagTypeString, Default: "",
			Description: "Kubernetes namespaces to monitor (comma-separated, empty = all)",
			ApplyTo:     func(c *Config, v string) error { c.K8sNamespaces = v; return nil },
		},
		{
			EnvName: "MAINTENANT_K8S_EXCLUDE_NAMESPACES", FlagName: "k8sExcludeNamespaces",
			Type: FlagTypeString, Default: "",
			Description: "Kubernetes namespaces to exclude (comma-separated)",
			ApplyTo:     func(c *Config, v string) error { c.K8sExcludeNS = v; return nil },
		},
		{
			EnvName: "MAINTENANT_STATUS_URL", FlagName: "statusUrl",
			Type: FlagTypeString, Default: "",
			Description: "Public status page URL advertised in notifications",
			ApplyTo:     func(c *Config, v string) error { c.StatusURL = v; return nil },
		},
		// Retention
		{
			EnvName: "MAINTENANT_RETENTION_SNAPSHOTS", FlagName: "retentionSnapshots",
			Type: FlagTypeDuration, Default: "48h",
			Description: "How long raw resource samples are kept",
			ApplyTo: func(c *Config, v string) error {
				d, err := time.ParseDuration(v)
				if err != nil {
					return err
				}
				c.Retention.Snapshots = d
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_RETENTION_INTERVAL", FlagName: "retentionInterval",
			Type: FlagTypeDuration, Default: "1h",
			Description: "How often the retention cleanup runs",
			ApplyTo: func(c *Config, v string) error {
				d, err := time.ParseDuration(v)
				if err != nil {
					return err
				}
				c.Retention.Interval = d
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_RETENTION_BATCH_SIZE", FlagName: "retentionBatchSize",
			Type: FlagTypeInt, Default: "1000",
			Description: "Rows deleted per retention batch",
			ApplyTo: func(c *Config, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return err
				}
				c.Retention.BatchSize = n
				return nil
			},
		},
		// Multi-host
		{
			EnvName: "MAINTENANT_MODE", FlagName: "mode",
			Type: FlagTypeString, Default: "embedded",
			Description: "Operating mode (embedded|server|agent)",
			ApplyTo:     func(c *Config, v string) error { c.Mode = v; return nil },
		},
		{
			EnvName: "MAINTENANT_SERVER", FlagName: "server",
			Type: FlagTypeString, Default: "",
			Description: "gRPC server URL to enrol against (agent mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.ServerURL = v; return nil },
		},
		{
			EnvName: "MAINTENANT_ENROLLMENT_TOKEN", FlagName: "enrollment-token",
			Type: FlagTypeString, Default: "", Sensitive: true,
			Description: "Enrollment token (agent mode, first boot)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.EnrollmentToken = v; return nil },
		},
		{
			EnvName: "MAINTENANT_LABEL", FlagName: "label",
			Type: FlagTypeString, Default: "",
			Description: "Display label for this agent (agent mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.Label = v; return nil },
		},
		{
			EnvName: "MAINTENANT_GRPC_LISTEN", FlagName: "grpc-listen",
			Type: FlagTypeString, Default: "127.0.0.1:8443",
			Description: "gRPC listen address (server mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.GRPCListen = v; return nil },
		},
		{
			EnvName: "MAINTENANT_GRPC_URL", FlagName: "grpc-url",
			Type: FlagTypeString, Default: "",
			Description: "Public gRPC URL handed to agents (server mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.GRPCPublicURL = v; return nil },
		},
		{
			EnvName: "MAINTENANT_GRPC_TLS_CERT", FlagName: "grpc-tls-cert",
			Type: FlagTypeString, Default: "",
			Description: "TLS certificate file for the gRPC listener (server mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.TLSCertFile = v; return nil },
		},
		{
			EnvName: "MAINTENANT_GRPC_TLS_KEY", FlagName: "grpc-tls-key",
			Type: FlagTypeString, Default: "",
			Description: "TLS key file for the gRPC listener (server mode)",
			ApplyTo:     func(c *Config, v string) error { c.MultiHost.TLSKeyFile = v; return nil },
		},
		{
			EnvName: "MAINTENANT_GRPC_INSECURE_SKIP_TLS_VERIFY", FlagName: "grpc-insecure-skip-tls-verify",
			Type: FlagTypeBool, Default: "false",
			Description: "Disable TLS verification against the server (debug only)",
			ApplyTo: func(c *Config, v string) error {
				c.MultiHost.InsecureSkipVerify = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_EMBEDDED_AGENT", FlagName: "embedded-agent",
			Type: FlagTypeBool, Default: "false",
			Description: "Also run a local agent (server mode, Pro)",
			ApplyTo: func(c *Config, v string) error {
				c.MultiHost.EmbeddedAgent = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_GRPC_TLS_INSECURE", FlagName: "grpc-tls-insecure",
			Type: FlagTypeBool, Default: "false",
			Description: "Serve gRPC as h2c, without TLS (trusted reverse proxy only)",
			ApplyTo: func(c *Config, v string) error {
				c.MultiHost.InsecureGRPC = parseTruthy(v)
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_AGENT_RATE_LIMIT_PER_SECOND", FlagName: "agentRateLimitPerSecond",
			Type: FlagTypeInt, Default: "1000",
			Description: "Maximum gRPC calls per second and per agent (server mode)",
			ApplyTo: func(c *Config, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return err
				}
				c.MultiHost.AgentRateLimitPerSecond = n
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_AGENT_STALE_THRESHOLD_SECONDS", FlagName: "agentStaleThresholdSeconds",
			Type: FlagTypeInt, Default: "60",
			Description: "Seconds without a heartbeat before an agent is stale (server mode)",
			ApplyTo: func(c *Config, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return err
				}
				c.MultiHost.AgentStaleThresholdSeconds = n
				return nil
			},
		},
		{
			EnvName: "MAINTENANT_DATA_DIR", FlagName: "data-dir",
			Type: FlagTypeString, Default: "/var/lib/maintenant",
			Description: "Directory holding the agent identity and liveness files (agent mode)",
			// Read straight from the environment where the agent starts, so the
			// flag has to land there too.
			ApplyTo: func(_ *Config, v string) error {
				return os.Setenv("MAINTENANT_DATA_DIR", v)
			},
		},
		{
			EnvName: "MAINTENANT_CA_CERT", FlagName: "ca-cert",
			Type: FlagTypeString, Default: "",
			Description: "PEM bundle of extra root CAs, added to the system store",
			ApplyTo:     func(c *Config, v string) error { c.CACertFile = v; return nil },
		},
		{
			EnvName: "MAINTENANT_DATABASE_URL", FlagName: "database-url",
			Type: FlagTypeString, Default: "", Sensitive: true,
			Description: "PostgreSQL connection string (server/embedded only; empty = SQLite)",
			ApplyTo:     func(c *Config, v string) error { c.DatabaseURL = v; return nil },
		},
		// Actions — read straight from the visited map by main, they configure
		// nothing.
		{
			FlagName: "copy-store-to", NoEnv: true,
			Type: FlagTypeString, Default: "",
			Description: "Copy this install into an empty PostgreSQL database, then exit",
			ApplyTo:     func(*Config, string) error { return nil },
		},
		{
			FlagName: "yes", NoEnv: true,
			Type: FlagTypeBool, Default: "false",
			Description: "Skip the confirmation prompt (for scripts)",
			ApplyTo:     func(*Config, string) error { return nil },
		},
	}

	// Validate registry invariants at init time (panics = internal bug)
	names := make(map[string]bool, len(Registry))
	envNames := make(map[string]bool, len(Registry))
	for _, spec := range Registry {
		if names[spec.FlagName] {
			panic("flags: duplicate FlagName: " + spec.FlagName)
		}
		names[spec.FlagName] = true
		if spec.NoEnv {
			if spec.EnvName != "" {
				panic("flags: NoEnv spec must not set EnvName: " + spec.FlagName)
			}
			continue
		}
		if envNames[spec.EnvName] {
			panic("flags: duplicate EnvName: " + spec.EnvName)
		}
		envNames[spec.EnvName] = true
		if !strings.HasPrefix(spec.EnvName, "MAINTENANT_") {
			panic("flags: EnvName must start with MAINTENANT_: " + spec.EnvName)
		}
	}

	Categories = []FlagCategory{
		{Name: "Server", Specs: specsFor("addr", "baseUrl", "corsOrigins")},
		{Name: "Storage", Specs: specsFor("db")},
		{Name: "Retention", Specs: specsFor("retentionSnapshots", "retentionInterval", "retentionBatchSize")},
		{Name: "Branding", Specs: specsFor("organisationName", "statusUrl")},
		{Name: "Runtime", Specs: specsFor("runtime")},
		{Name: "Logging", Specs: specsFor("logLevel")},
		{Name: "HTTP", Specs: specsFor("maxBodySize")},
		{Name: "Updates", Specs: specsFor("updateInterval")},
		{Name: "Security", Specs: specsFor("securityScoreThreshold", "disableTelemetry", "allowPrivateWebhooks")},
		{Name: "Pro", Specs: specsFor("licenseKey")},
		{Name: "SMTP", Specs: specsFor("smtpHost", "smtpPort", "smtpUsername", "smtpPassword", "smtpFrom")},
		{Name: "MCP", Specs: specsFor(
			"mcp", "mcpClientId", "mcpClientSecret",
			"mcpAllowedRedirectUris", "mcpAllowUnauthenticated",
		)},
		{Name: "Kubernetes", Specs: specsFor("k8sNamespaces", "k8sExcludeNamespaces")},
		{Name: "Multi-host", Specs: specsFor(
			"mode", "server", "enrollment-token", "label",
			"grpc-listen", "grpc-url", "grpc-tls-cert", "grpc-tls-key",
			"grpc-tls-insecure", "grpc-insecure-skip-tls-verify",
			"agentRateLimitPerSecond", "agentStaleThresholdSeconds",
			"embedded-agent", "ca-cert", "data-dir",
		)},
		{Name: "Storage (PostgreSQL)", Specs: specsFor("database-url", "copy-store-to", "yes")},
	}
}

func specsFor(flagNames ...string) []FlagSpec {
	out := make([]FlagSpec, 0, len(flagNames))
	for _, name := range flagNames {
		for _, s := range Registry {
			if s.FlagName == name {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// ParseFlagsOrDie parses args using the Registry and returns a map of
// explicitly set flag names to their string values.
func ParseFlagsOrDie(args []string) (map[string]string, error) {
	fs := flag.NewFlagSet("maintenant", flag.ContinueOnError)
	fs.Usage = func() {} // suppress default usage; handled by PrintHelp

	// Booleans are declared as booleans, not strings: `--yes` and
	// `--embedded-agent` are used without a value, and a string flag would
	// swallow the next argument.
	for _, spec := range Registry {
		if spec.Type == FlagTypeBool {
			fs.Bool(spec.FlagName, parseTruthy(spec.Default), spec.Description)
			continue
		}
		fs.String(spec.FlagName, spec.Default, spec.Description)
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	visited := make(map[string]string)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = f.Value.String()
	})
	return visited, nil
}

// MergeArgsIntoConfig applies visited CLI flags into cfg, overriding env values.
func MergeArgsIntoConfig(cfg *Config, visited map[string]string) error {
	for name, value := range visited {
		spec, ok := specByFlagName(name)
		if !ok {
			continue
		}
		if err := spec.ApplyTo(cfg, value); err != nil {
			return fmt.Errorf("flag --%s: %w", name, err)
		}
	}
	return nil
}

func specByFlagName(name string) (FlagSpec, bool) {
	for _, s := range Registry {
		if s.FlagName == name {
			return s, true
		}
	}
	return FlagSpec{}, false
}

// PrintHelp writes the formatted help text to w.
func PrintHelp(w io.Writer, _ Config) {
	fmt.Fprintln(w, "maintenant — infrastructure monitoring (single binary)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  maintenant [FLAGS]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags are equivalent to MAINTENANT_* environment variables.")
	fmt.Fprintln(w, "Precedence: CLI flag > environment variable > default.")
	fmt.Fprintln(w)

	for _, cat := range Categories {
		fmt.Fprintf(w, "%s:\n", cat.Name)
		for _, spec := range cat.Specs {
			typeSuffix := flagTypeSuffix(spec)
			fmt.Fprintf(w, "  --%s%s\n", spec.FlagName, typeSuffix)
			fmt.Fprintf(w, "      %s", spec.Description)
			if spec.Default != "" && !spec.Sensitive {
				fmt.Fprintf(w, " (default: %s)", spec.Default)
			}
			fmt.Fprintln(w)
			if !spec.NoEnv {
				fmt.Fprintf(w, "      env: %s", spec.EnvName)
				if spec.Sensitive {
					fmt.Fprint(w, "  default: <unset>")
				}
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "Special:")
	fmt.Fprintln(w, "  --help, -h         Show this help and exit")
	fmt.Fprintln(w, "  --version, -v      Show version and exit")
	fmt.Fprintln(w, "  --mcp-stdio        Run as MCP stdio server (advanced)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Documentation: https://docs.maintenant.dev")
	fmt.Fprintln(w, "Install via script: curl -fsSL https://install.maintenant.dev | sudo bash")
}

func flagTypeSuffix(spec FlagSpec) string {
	switch spec.Type {
	case FlagTypeString:
		return " <string>"
	case FlagTypeInt:
		return " <int>"
	case FlagTypeBool:
		return ""
	case FlagTypeDuration:
		return " <duration>"
	}
	return ""
}
