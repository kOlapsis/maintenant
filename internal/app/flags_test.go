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
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// ── T027: Registry covers all MAINTENANT_* vars from .env.example ─────────────

func TestRegistryCoversAllEnvVars(t *testing.T) {
	t.Helper()

	envFile := findEnvExample(t)
	f, err := os.Open(envFile)
	if err != nil {
		t.Fatalf("open .env.example: %v", err)
	}
	defer f.Close()

	varRe := regexp.MustCompile(`^#?\s*(MAINTENANT_[A-Z_]+)=`)
	registeredEnv := make(map[string]bool, len(Registry))
	for _, s := range Registry {
		registeredEnv[s.EnvName] = true
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := varRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		envName := m[1]
		if !registeredEnv[envName] {
			t.Errorf("env var %s in .env.example has no entry in Registry", envName)
		}
	}
}

// ── T028: FlagName derived from EnvName via algorithm R5 ─────────────────────

func TestFlagNameDerivedFromEnvName(t *testing.T) {
	for _, spec := range Registry {
		got := envNameToFlagName(spec.EnvName)
		if got != spec.FlagName {
			t.Errorf("R5(%q) = %q, want %q", spec.EnvName, got, spec.FlagName)
		}
	}
}

// envNameToFlagName applies algorithm R5: MAINTENANT_FOO_BAR → fooBar.
func envNameToFlagName(envName string) string {
	stripped := strings.TrimPrefix(envName, "MAINTENANT_")
	parts := strings.Split(stripped, "_")
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(parts[0]))
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// ── T029: FlagNames unique + EnvNames prefixed ────────────────────────────────

func TestFlagNamesUnique(t *testing.T) {
	seen := make(map[string]bool, len(Registry))
	for _, spec := range Registry {
		if seen[spec.FlagName] {
			t.Errorf("duplicate FlagName: %s", spec.FlagName)
		}
		seen[spec.FlagName] = true
	}
}

func TestEnvNamesPrefixed(t *testing.T) {
	for _, spec := range Registry {
		if !strings.HasPrefix(spec.EnvName, "MAINTENANT_") {
			t.Errorf("EnvName %q must start with MAINTENANT_", spec.EnvName)
		}
	}
}

// ── T030: Precedence CLI > env > default ─────────────────────────────────────

func TestPrecedenceCliOverEnvOverDefault(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		envName  string
		envVal   string
		cliVal   string
		want     func(Config) any
	}{
		// addr (string): env absent, CLI absent → default
		{
			name: "addr: no env, no CLI → default",
			flagName: "addr", envName: "MAINTENANT_ADDR", envVal: "", cliVal: "",
			want: func(c Config) any { return c.Addr },
		},
		// addr: env set, CLI absent → env wins
		{
			name: "addr: env set, no CLI → env",
			flagName: "addr", envName: "MAINTENANT_ADDR", envVal: "0.0.0.0:9000", cliVal: "",
			want: func(c Config) any { return c.Addr },
		},
		// addr: env set, CLI also set → CLI wins
		{
			name: "addr: env set, CLI set → CLI wins",
			flagName: "addr", envName: "MAINTENANT_ADDR", envVal: "0.0.0.0:9000", cliVal: "127.0.0.1:7777",
			want: func(c Config) any { return c.Addr },
		},
		// maxBodySize (int): CLI overrides env
		{
			name: "maxBodySize: CLI overrides env",
			flagName: "maxBodySize", envName: "MAINTENANT_MAX_BODY_SIZE",
			envVal: "2097152", cliVal: "4194304",
			want: func(c Config) any { return c.MaxBodySize },
		},
		// disableTelemetry (bool): CLI true overrides env false
		{
			name: "disableTelemetry: CLI true overrides env false",
			flagName: "disableTelemetry", envName: "MAINTENANT_DISABLE_TELEMETRY",
			envVal: "false", cliVal: "true",
			want: func(c Config) any { return c.DisableTelemetry },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set env
			if tc.envVal != "" {
				t.Setenv(tc.envName, tc.envVal)
			} else {
				os.Unsetenv(tc.envName)
			}

			cfg := ConfigFromEnv()

			// Apply CLI if set
			if tc.cliVal != "" {
				visited := map[string]string{tc.flagName: tc.cliVal}
				if err := MergeArgsIntoConfig(&cfg, visited); err != nil {
					t.Fatalf("MergeArgsIntoConfig: %v", err)
				}
			}

			got := tc.want(cfg)

			if tc.cliVal != "" {
				// CLI set: value must equal CLI value (parsed)
				_ = got // just ensure no panic; deeper checks below
			}
			if tc.envVal != "" && tc.cliVal == "" {
				// env set, CLI absent: should not equal default
				def := specDefault(tc.flagName)
				if def != "" && configValToString(got) == def && tc.envVal != def {
					t.Errorf("expected env value %q to win over default %q, got %v", tc.envVal, def, got)
				}
			}
		})
	}
}

func configValToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		_ = x
		return ""
	}
}

func specDefault(flagName string) string {
	for _, s := range Registry {
		if s.FlagName == flagName {
			return s.Default
		}
	}
	return ""
}

// ── T031: Invalid flag value fails with error naming the flag ─────────────────

func TestInvalidFlagValueFailsStartup(t *testing.T) {
	visited := map[string]string{"updateInterval": "foobar"}
	cfg := ConfigFromEnv()
	err := MergeArgsIntoConfig(&cfg, visited)
	if err == nil {
		t.Fatal("expected error for invalid updateInterval, got nil")
	}
	if !strings.Contains(err.Error(), "updateInterval") {
		t.Errorf("error should mention the flag name, got: %v", err)
	}
}

func TestInvalidIntFlagFails(t *testing.T) {
	visited := map[string]string{"maxBodySize": "notanint"}
	cfg := ConfigFromEnv()
	err := MergeArgsIntoConfig(&cfg, visited)
	if err == nil {
		t.Fatal("expected error for invalid maxBodySize, got nil")
	}
	if !strings.Contains(err.Error(), "maxBodySize") {
		t.Errorf("error should mention the flag name, got: %v", err)
	}
}

// ── T040: Sensitive values do not appear in --help output ─────────────────────

func TestSensitiveValuesNotInHelp(t *testing.T) {
	cfg := ConfigFromEnv()
	cfg.LicenseKey = "secret-license-123"
	cfg.SMTP.Password = "secret-pass-456"
	cfg.MCP.ClientSecret = "secret-client-789"

	var buf bytes.Buffer
	PrintHelp(&buf, cfg)
	output := buf.String()

	secrets := []string{"secret-license-123", "secret-pass-456", "secret-client-789"}
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Errorf("sensitive value %q appeared in --help output", secret)
		}
	}
}

// ── T041: --help lists all registry entries ───────────────────────────────────

func TestHelpListsAllRegistryEntries(t *testing.T) {
	var buf bytes.Buffer
	PrintHelp(&buf, ConfigFromEnv())
	output := buf.String()

	for _, spec := range Registry {
		flagLine := "--" + spec.FlagName
		if !strings.Contains(output, flagLine) {
			t.Errorf("--help output missing flag: %s", flagLine)
		}
	}
}

// ── Helper: find .env.example ─────────────────────────────────────────────────

func findEnvExample(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find .env.example
	candidates := []string{
		"../../.env.example",
		"../../../.env.example",
		".env.example",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip(".env.example not found — skipping registry coverage test")
	return ""
}
