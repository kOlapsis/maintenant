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

package v1

import (
	"strings"
	"testing"
)

func TestBuildInstallTemplates_ReturnsFourModes(t *testing.T) {
	const (
		serverURL = "grpcs://monitoring.example.com:8443"
		token     = "mnt_enr_testtokenabc123"
	)

	tpl := buildInstallTemplates(serverURL, token)

	wantKeys := []string{"standalone", "docker_run", "docker_compose", "kubernetes"}
	for _, k := range wantKeys {
		v, ok := tpl[k]
		if !ok {
			t.Errorf("missing template key %q", k)
			continue
		}
		if v == "" {
			t.Errorf("template %q is empty", k)
		}
		if !strings.Contains(v, serverURL) {
			t.Errorf("template %q does not contain server URL %q", k, serverURL)
		}
		if !strings.Contains(v, token) {
			t.Errorf("template %q does not contain token", k)
		}
	}

	if len(tpl) != len(wantKeys) {
		t.Errorf("unexpected number of templates: got %d, want %d", len(tpl), len(wantKeys))
	}
}

func TestBuildInstallStandalone_StartsWithCurl(t *testing.T) {
	out := buildInstallStandalone("grpcs://h:8443", "mnt_enr_xyz")
	if !strings.HasPrefix(out, "curl -fsSL https://install.maintenant.dev") {
		t.Errorf("standalone template should start with curl install.sh invocation, got:\n%s", out)
	}
	if !strings.Contains(out, "--mode=agent") {
		t.Errorf("standalone template missing --mode=agent")
	}
}

func TestBuildInstallDockerRun_ContainsDockerSocketMount(t *testing.T) {
	out := buildInstallDockerRun("grpcs://h:8443", "mnt_enr_xyz")
	if !strings.HasPrefix(out, "docker run -d") {
		t.Errorf("docker_run template should start with `docker run -d`, got:\n%s", out)
	}
	if !strings.Contains(out, "/var/run/docker.sock:/var/run/docker.sock:ro") {
		t.Errorf("docker_run template missing docker.sock mount")
	}
	if !strings.Contains(out, "ghcr.io/kolapsis/maintenant:latest") {
		t.Errorf("docker_run template missing image reference")
	}
}

func TestBuildInstallDockerCompose_HasServicesBlock(t *testing.T) {
	out := buildInstallDockerCompose("grpcs://h:8443", "mnt_enr_xyz")
	if !strings.HasPrefix(out, "services:\n") {
		t.Errorf("docker_compose template should start with `services:`, got:\n%s", out)
	}
	if !strings.Contains(out, "maintenant-agent:") {
		t.Errorf("docker_compose template missing service name")
	}
	if !strings.Contains(out, "volumes:") {
		t.Errorf("docker_compose template missing volumes block")
	}
}

func TestBuildInstallKubernetes_HasDaemonSetAndSecret(t *testing.T) {
	out := buildInstallKubernetes("grpcs://h:8443", "mnt_enr_xyz")
	if !strings.Contains(out, "kind: DaemonSet") {
		t.Errorf("kubernetes template missing DaemonSet kind")
	}
	if !strings.Contains(out, "kind: Secret") {
		t.Errorf("kubernetes template missing Secret kind")
	}
	if !strings.Contains(out, "kind: ClusterRole") {
		t.Errorf("kubernetes template missing ClusterRole (RBAC)")
	}
	if !strings.Contains(out, "maintenant-agent-enrollment") {
		t.Errorf("kubernetes template missing enrollment secret name")
	}
	if !strings.Contains(out, "--runtime=kubernetes") {
		t.Errorf("kubernetes template should pin runtime to kubernetes")
	}
}
