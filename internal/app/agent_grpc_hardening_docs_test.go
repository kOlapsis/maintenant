// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import "testing"

// Documentation floors of 046-agent-grpc-hardening. The wording itself is reviewed by hand.

var (
	agentSetupPage = []string{"docs", "guides", "agent-setup.md"}
	multihostPage  = []string{"docs", "features", "multihost.md"}
)

// FR-006, US1 scenario 8: the three pages describe the persistent certificate and the
// fingerprint, and no longer present the in-memory certificate as the fallback.
func TestAgentDocsDescribeThePersistentCertificateAndTheFingerprint(t *testing.T) {
	for _, page := range [][]string{agentSetupPage, multihostPage, secPage} {
		mentions(t, page, "--grpc-server-fingerprint")
		mentions(t, page, "grpc-server.crt")
		matches(t, page, `(?i)fingerprint`, "the fingerprint is explained")
	}
	mentions(t, agentSetupPage, "MAINTENANT_GRPC_SERVER_FINGERPRINT")
	mentionsNot(t, agentSetupPage, "generates a self-signed cert in-memory")
	mentionsNot(t, agentSetupPage, "Agents must pass `--grpc-insecure-skip-tls-verify`")
	mentionsNot(t, multihostPage, "generates a self-signed certificate in-memory")
	mentionsNot(t, secPage, "generates a self-signed certificate in memory at each start")
}

// FR-006, FR-005: --grpc-insecure-skip-tls-verify stays documented, as a workaround to avoid.
func TestAgentDocsKeepSkipVerifyAsAWorkaround(t *testing.T) {
	for _, page := range [][]string{agentSetupPage, multihostPage} {
		mentions(t, page, "--grpc-insecure-skip-tls-verify")
	}
}

// FR-016: the multi-host page and the security guide describe the gRPC limits.
func TestMultihostAndSecurityDocsDescribeTheGRPCLimits(t *testing.T) {
	for _, page := range [][]string{multihostPage, secPage} {
		matches(t, page, `10[ ,.]?000`, "the inventory bound is documented")
		matches(t, page, `256\s+labels`, "the label bound is documented")
		matches(t, page, `1[ ,.]?048[ ,.]?576|1 ?Mi?[Bo]`, "the message size limit is documented")
		matches(t, page, `(?i)(auth\w*|challenge)[^\n]*\b30\s*(s\b|sec)|\b30\s*(s\b|sec)[^\n]*(auth\w*|challenge)`, "the authentication delay is documented")
		matches(t, page, `(?i)(10|ten)\s*(per|/|a)\s*min`, "the per-address quota is documented")
	}
}
