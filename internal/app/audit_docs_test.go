// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13 and C14: the documentation says what the product does.

func readDoc(t *testing.T, rel ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, rel...)...))
	require.NoError(t, err)
	return string(data)
}

// sectionOf returns the body of the markdown section whose heading contains title, up to the next heading of the same or a higher level.
func sectionOf(t *testing.T, doc, title string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#") || !strings.Contains(line, title) {
			continue
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		var body []string
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(next, "#") {
				if l := len(next) - len(strings.TrimLeft(next, "#")); l <= level {
					break
				}
			}
			body = append(body, next)
		}
		return strings.Join(body, "\n")
	}
	t.Fatalf("no heading containing %q", title)
	return ""
}

// The documents are long: these keep the failure output to the point that is missing.
func docHas(t *testing.T, doc, needle, msg string) {
	t.Helper()
	assert.True(t, strings.Contains(doc, needle), "%s: %q not found", msg, needle)
}

func docLacks(t *testing.T, doc, needle, msg string) {
	t.Helper()
	assert.False(t, strings.Contains(doc, needle), "%s: %q still present", msg, needle)
}

func docMatches(t *testing.T, doc, pattern, msg string) {
	t.Helper()
	assert.True(t, regexp.MustCompile(pattern).MatchString(doc), "%s: nothing matches %s", msg, pattern)
}

var memoryGuides = [][]string{
	{"README.md"},
	{"docs", "guides", "scaleway.md"},
	{"docs", "guides", "hetzner.md"},
	{"docs", "guides", "digitalocean.md"},
	{"docs", "guides", "ovhcloud.md"},
	{"docs", "guides", "vultr.md"},
}

const about100MB = `(?i)\b100 ?M(i?B|o)\b`

// FR-014, US5 scenario 1: nobody is promised "under 30 MB" any more; the figure after days on a busy host is stated.
func TestMemoryPromiseIsMeasured(t *testing.T) {
	for _, rel := range memoryGuides {
		t.Run(filepath.Join(rel...), func(t *testing.T) {
			doc := readDoc(t, rel...)
			lower := strings.ToLower(doc)
			docLacks(t, lower, "30 mb", "the old promise")
			docLacks(t, lower, "30mb", "the old promise")
			docMatches(t, doc, about100MB, "the reader must know to count about 100 MB after several days on a busy host")
		})
	}
}

// FR-014, US5 scenario 2: the architecture document carries the protocol and the figures.
func TestArchitectureDescribesTheMemoryMeasurement(t *testing.T) {
	section := sectionOf(t, readDoc(t, "docs", "architecture.md"), "Memory footprint")
	lower := strings.ToLower(section)

	docHas(t, lower, "docker stats", "the measurement tool")
	docMatches(t, section, `(?i)\b32 ?M(i?B|o)\b`, "the figure one hour after a restart")
	docMatches(t, section, about100MB, "the figure after several days on a busy host")
	docHas(t, lower, "hour", "32 MB one hour after a restart")
	docHas(t, lower, "days", "about 100 MB after several days")
	docHas(t, lower, "growth", "the growth is said to be under study")
}

// FR-013, US4 scenario 5: the channel test body and answer, name on standalone endpoints only, /api/v1/updates/hosts.
func TestAPIReferenceTellsTheTruth(t *testing.T) {
	doc := readDoc(t, "docs", "api", "reference.md")

	channels := sectionOf(t, doc, "Notification Channels")
	docMatches(t, channels, `"event":\s*"test"`, "the body of the channel test")
	docMatches(t, channels, `"alert":\s*\{`, "the body of the channel test")
	docHas(t, channels, `"timestamp"`, "the body of the channel test")
	docHas(t, channels, "response_code", "the answer of the channel test")

	endpoints := sectionOf(t, doc, "Endpoints")
	docMatches(t, endpoints, "(?i)`name`[^\n.]{0,80}standalone", "name exists for standalone endpoints only")

	docHas(t, doc, "`/api/v1/updates/hosts`", "the per-host updates route")
}

// FR-003: the documentation follows the SMTP change.
func TestSMTPDocsNameTheClearTextOptIn(t *testing.T) {
	for _, rel := range [][]string{
		{"docs", "features", "alerts.md"},
		{"docs", "features", "status-page.md"},
		{"docs", "security.md"},
		{"docs", "getting-started", "configuration.md"},
	} {
		t.Run(filepath.Join(rel...), func(t *testing.T) {
			docHas(t, readDoc(t, rel...), "MAINTENANT_SMTP_ALLOW_PLAINTEXT", "the clear-text opt-in")
		})
	}
}
