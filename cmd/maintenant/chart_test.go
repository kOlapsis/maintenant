// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chartTemplate(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "helm", "maintenant", "templates", name))
	require.NoError(t, err)
	return string(raw)
}

func TestChartKeepsTheDataClaimOnUninstall(t *testing.T) {
	assert.Contains(t, chartTemplate(t, "pvc.yaml"), "helm.sh/resource-policy: keep")
	assert.Contains(t, chartTemplate(t, "NOTES.txt"), "kubectl delete pvc")
}

func TestChartPodFitsTheRestrictedPodSecurityLevel(t *testing.T) {
	for _, manifest := range []string{
		chartTemplate(t, "deployment.yaml"),
		func() string {
			raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "kubernetes", "deployment.yaml"))
			require.NoError(t, err)
			return string(raw)
		}(),
	} {
		assert.Contains(t, manifest, "seccompProfile:\n          type: RuntimeDefault")
		assert.Contains(t, manifest, "runAsNonRoot: true")
		assert.Contains(t, manifest, "allowPrivilegeEscalation: false")
	}
}

func TestChartDerivesTheBaseURLFromTheIngress(t *testing.T) {
	deployment := chartTemplate(t, "deployment.yaml")

	assert.Contains(t, deployment, "{{- if and .Values.ingress.enabled (not $baseURLSet) }}\n            - name: MAINTENANT_BASE_URL")
	assert.Contains(t, deployment, `(ternary "https" "http" (not (empty .Values.ingress.tls))) .Values.ingress.host`)
}

func TestChartRBACFollowsRBACCreate(t *testing.T) {
	rbac := chartTemplate(t, "rbac.yaml")

	assert.Contains(t, rbac, "{{- if .Values.rbac.create }}")
	assert.Contains(t, rbac, `name: {{ include "maintenant.serviceAccountName" . }}`)
}
