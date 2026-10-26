// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Documentation floors of 045-mcp-hardening. The wording itself is reviewed by hand.

func mentions(t *testing.T, page []string, needle string) {
	t.Helper()
	assert.True(t, strings.Contains(repoFile(t, page...), needle), "%s must mention %q", strings.Join(page, "/"), needle)
}

func mentionsNot(t *testing.T, page []string, needle string) {
	t.Helper()
	assert.False(t, strings.Contains(repoFile(t, page...), needle), "%s must no longer say %q", strings.Join(page, "/"), needle)
}

func matches(t *testing.T, page []string, pattern string, what string) {
	t.Helper()
	assert.True(t, regexp.MustCompile(pattern).MatchString(repoFile(t, page...)), "%s: %s", strings.Join(page, "/"), what)
}

var (
	mcpPage    = []string{"docs", "features", "mcp.md"}
	agentsPage = []string{"docs", "guides", "ai-agents.md"}
	configPage = []string{"docs", "getting-started", "configuration.md"}
	secPage    = []string{"docs", "security.md"}
)

// FR-003, US1 scenario 4: the MCP page documents read-only mode and what /mcp without it amounts to.
func TestMCPDocsDescribeReadOnlyModeAndTheAdminAPIEquivalence(t *testing.T) {
	mentions(t, mcpPage, "MAINTENANT_MCP_READ_ONLY")
	mentions(t, mcpPage, "admin API")
}

// FR-014: the MCP page and the security guide describe the scopes and their effect.
func TestMCPAndSecurityDocsDescribeTheScopes(t *testing.T) {
	for _, page := range [][]string{mcpPage, secPage} {
		mentions(t, page, "read write")
		mentions(t, page, "scope")
	}
	mentions(t, mcpPage, "invalid_scope")
}

// FR-023: the MCP page and the agents guide describe the framing and the limits of container logs.
func TestMCPAndAgentDocsDescribeTheLogFraming(t *testing.T) {
	matches(t, mcpPage, `(?i)untrusted`, "the log preamble is explained")
	matches(t, mcpPage, `4[ ,.]?096|4 ?Ki?B`, "the per-line limit is documented")
	matches(t, mcpPage, `1[ ,.]?048[ ,.]?576|1 ?Mi?B`, "the body limit is documented")
	matches(t, agentsPage, `(?i)untrusted`, "the guide says container output is untrusted")
}

// FR-032: the MCP page and the configuration reference document MAINTENANT_MCP_SOCKET.
func TestMCPAndConfigurationDocsDocumentTheSocketVariable(t *testing.T) {
	mentions(t, mcpPage, "MAINTENANT_MCP_SOCKET")
	mentions(t, configPage, "MAINTENANT_MCP_SOCKET")
	mentions(t, configPage, "MAINTENANT_MCP_READ_ONLY")
}

// FR-041, US5 scenario 3: the agents guide no longer suggests acknowledge_alert and says why.
func TestAgentDocsAdviseAgainstWriteToolsForALogReadingAgent(t *testing.T) {
	mentionsNot(t, agentsPage, "Add `acknowledge_alert` to `READ_TOOLS`")
	mentionsNot(t, agentsPage, "if you want the agent to acknowledge")
	mentions(t, agentsPage, "acknowledge_alert")
	matches(t, agentsPage, `(?i)instruction`, "the guide explains the injection risk")
}
