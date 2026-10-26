// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 043-public-surfaces-hardening on the shipped manifests and the documentation.
// The documentation checks are floors: the wording itself is reviewed by hand.

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	require.NoError(t, err)
	return string(data)
}

// FR-034, US4 scenario 6: every HTTP probe of the shipped manifests sends Host: localhost.
func TestKubernetesProbesSendAnAcceptedHost(t *testing.T) {
	for _, file := range []string{
		filepath.Join("deploy", "kubernetes", "deployment.yaml"),
		filepath.Join("deploy", "helm", "maintenant", "templates", "deployment.yaml"),
	} {
		t.Run(file, func(t *testing.T) {
			manifest := repoFile(t, file)
			probes := 0
			for _, probe := range []string{"startupProbe:", "livenessProbe:", "readinessProbe:"} {
				start := strings.Index(manifest, probe)
				require.GreaterOrEqual(t, start, 0, "%s is missing", probe)
				block := manifest[start:]
				if end := strings.Index(block[len(probe):], "Probe:"); end >= 0 {
					block = block[:len(probe)+end]
				}
				if end := strings.Index(block, "resources:"); end >= 0 {
					block = block[:end]
				}
				assert.Contains(t, block, "path: /api/v1/health", "%s probes the health route", probe)
				assert.Contains(t, block, "httpHeaders:", "%s sets a Host header", probe)
				assert.Contains(t, block, "name: Host", "%s sets a Host header", probe)
				assert.Contains(t, block, "value: localhost", "%s sends Host: localhost", probe)
				probes++
			}
			assert.Equal(t, 3, probes)
		})
	}
}

// FR-005, FR-023: the status page documentation describes the public form of a monitor and restricts the Traefik example.
func TestStatusPageDocsDescribeThePublicFormAndRestrictTheSubdomainExample(t *testing.T) {
	doc := repoFile(t, "docs", "features", "status-page.md")

	assert.Contains(t, doc, "label", "the public label of a monitor is documented")
	assert.NotContains(t, doc, "traefik.http.routers.maintenant-status.rule: \"Host(`status.example.com`)\"",
		"the status subdomain router must not match the whole host")
	assert.Contains(t, doc, "PathPrefix(`/status", "the subdomain router is restricted to the public paths")
	assert.Contains(t, doc, "/assets/", "the assets stay reachable on the subdomain")
	assert.Contains(t, doc, "manifest.webmanifest", "the installable app files stay reachable on the subdomain")
	assert.Contains(t, doc, "404", "the server refuses the other paths on its own")
}

// FR-014: the heartbeat documentation treats the token as a secret and explains the rotation.
func TestHeartbeatDocsExplainTheSecretTokenAndItsRotation(t *testing.T) {
	doc := repoFile(t, "docs", "features", "heartbeats.md")
	assert.Contains(t, doc, "secret")
	assert.Contains(t, doc, "rotate-token")
	assert.Contains(t, doc, "ping_token")
}

// FR-014, FR-023, FR-035, FR-044: the security guide replaces "delete and recreate" by the rotation,
// explains the Host check, points to the status host rule and splits the public rate limits.
func TestSecurityDocsCoverTheHardenedPublicSurfaces(t *testing.T) {
	doc := repoFile(t, "docs", "security.md")

	assert.NotContains(t, doc, "by deleting and recreating it", "rotation replaces delete and recreate")
	assert.Contains(t, doc, "rotate-token")
	assert.Contains(t, doc, "MAINTENANT_ALLOWED_HOSTS")
	assert.Contains(t, doc, "421")
	assert.Contains(t, doc, "rebinding")
	assert.Contains(t, doc, "MAINTENANT_STATUS_URL", "the status host rule is referenced")
	assert.NotContains(t, doc, "| Public | `/ping/`, every `/status/` route", "pings no longer share the status page bucket")
	assert.Contains(t, doc, "60", "the per-token ping quota is stated")
}

// FR-035: the configuration reference lists the new variable.
func TestConfigurationDocsListAllowedHosts(t *testing.T) {
	doc := repoFile(t, "docs", "getting-started", "configuration.md")
	assert.Contains(t, doc, "MAINTENANT_ALLOWED_HOSTS")
}
