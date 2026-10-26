// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// Acceptance tests of 046-agent-grpc-hardening, US1 scenarios 3 and 7, FR-003: the
// enrollment token response carries the served certificate's fingerprint and the
// install templates pin it when the certificate is self-signed.

// certSessions is the live registry as the handler sees it, reporting a given certificate.
type certSessions struct {
	fingerprint string
	selfSigned  bool
}

func (certSessions) IsConnected(string) bool { return false }
func (certSessions) Close(string, string)    {}
func (s certSessions) GRPCServerCertificate() (string, bool) {
	return s.fingerprint, s.selfSigned
}

func newCertTestHandler(t *testing.T, fingerprint string, selfSigned bool) *AgentHandler {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return extension.Pro }
	t.Cleanup(func() { extension.CurrentEdition = prev })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	return NewAgentHandler(store.NewAgentStore(db), certSessions{fingerprint, selfSigned}, nil, logger,
		"grpcs://example.test:8443", "127.0.0.1:8443", time.Minute, nil)
}

const (
	fingerprintA = "9f2e1d0c3b4a59687766554433221100ffeeddccbbaa99887766554433221100"
	fingerprintB = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
)

func templatesOf(t *testing.T, out map[string]any) map[string]string {
	t.Helper()
	raw, ok := out["install_templates"].(map[string]any)
	require.True(t, ok, "install_templates must be an object")
	templates := map[string]string{}
	for name, tmpl := range raw {
		s, ok := tmpl.(string)
		require.True(t, ok, "template %q must be a string", name)
		templates[name] = s
	}
	require.ElementsMatch(t, []string{"standalone", "docker_run", "docker_compose", "kubernetes"}, keys(templates))
	return templates
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// kubernetesFingerprint returns what the generated Deployment gives the agent as a fingerprint,
// through MAINTENANT_GRPC_SERVER_FINGERPRINT or --grpc-server-fingerprint.
func kubernetesFingerprint(t *testing.T, manifest string) string {
	t.Helper()
	dep := decodeOne[appsv1.Deployment](t, kubernetesManifest(t, manifest), "Deployment")
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]
	for _, e := range c.Env {
		if e.Name == "MAINTENANT_GRPC_SERVER_FINGERPRINT" {
			return e.Value
		}
	}
	for i, a := range c.Args {
		if v, ok := strings.CutPrefix(a, "--grpc-server-fingerprint="); ok {
			return v
		}
		if a == "--grpc-server-fingerprint" && i+1 < len(c.Args) {
			return c.Args[i+1]
		}
	}
	return ""
}

// US1 scenario 3, FR-003: self-signed certificate, every template pins the fingerprint.
func TestCreateEnrollmentToken_SelfSignedCertificateIsPinnedInEveryTemplate(t *testing.T) {
	for _, fp := range []string{fingerprintA, fingerprintB} {
		t.Run(fp[:8], func(t *testing.T) {
			h := newCertTestHandler(t, fp, true)
			out := createToken(t, h)

			assert.Equal(t, fp, out["grpc_server_fingerprint_sha256"])
			assert.Equal(t, true, out["grpc_server_self_signed"])

			templates := templatesOf(t, out)
			token := out["token"].(string)
			pin := regexp.MustCompile(`--grpc-server-fingerprint[= ]` + fp + `\b`)
			for _, name := range []string{"standalone", "docker_run", "docker_compose"} {
				assert.Regexp(t, pin, templates[name], "template %q must pass the fingerprint", name)
				assert.Contains(t, templates[name], token, "template %q still carries the token", name)
			}
			assert.Equal(t, fp, kubernetesFingerprint(t, templates["kubernetes"]), "the Deployment must give the agent the fingerprint")
			for name, tmpl := range templates {
				assert.NotContains(t, tmpl, "grpc-insecure-skip-tls-verify", "template %q must not disable verification", name)
			}
		})
	}
}

// US1 scenario 7, FR-003: provided certificate, fingerprint reported, no pin in the templates.
func TestCreateEnrollmentToken_ProvidedCertificateHasNoPinInTemplates(t *testing.T) {
	h := newCertTestHandler(t, fingerprintB, false)
	out := createToken(t, h)

	assert.Equal(t, fingerprintB, out["grpc_server_fingerprint_sha256"])
	assert.Equal(t, false, out["grpc_server_self_signed"])

	for name, tmpl := range templatesOf(t, out) {
		assert.NotContains(t, strings.ToLower(tmpl), "fingerprint", "template %q must rely on the trust chain", name)
		assert.NotContains(t, tmpl, "grpc-insecure-skip-tls-verify", "template %q must not disable verification", name)
		assert.Contains(t, tmpl, out["token"].(string))
	}
}

// Edge case "MAINTENANT_GRPC_TLS_INSECURE=true": no fingerprint, no pin.
func TestCreateEnrollmentToken_PlaintextListenerHasNoFingerprint(t *testing.T) {
	h := newCertTestHandler(t, "", false)
	out := createToken(t, h)

	assert.Equal(t, "", out["grpc_server_fingerprint_sha256"])
	assert.Equal(t, false, out["grpc_server_self_signed"])
	for name, tmpl := range templatesOf(t, out) {
		assert.NotContains(t, strings.ToLower(tmpl), "fingerprint", "template %q has nothing to pin", name)
		assert.NotContains(t, tmpl, "grpc-insecure-skip-tls-verify", name)
	}
}
