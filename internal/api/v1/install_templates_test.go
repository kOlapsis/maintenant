// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/kolapsis/maintenant/internal/kubernetes"
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

func TestBuildInstallStandalone_PipesTheInstallerInAgentMode(t *testing.T) {
	out := buildInstallStandalone("grpcs://h:8443", "mnt_enr_xyz")

	assert.True(t, strings.HasPrefix(out, "curl -fsSL https://install.maintenant.dev | sudo bash -s -- "), out)
	assert.Contains(t, out, "--mode agent")
	assert.Contains(t, out, "--server grpcs://h:8443")
	assert.Contains(t, out, "--enrollment-token mnt_enr_xyz")
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
	if !strings.Contains(out, "/etc/os-release:/host/etc/os-release:ro") {
		t.Errorf("docker_run template missing host os-release mount")
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
	if !strings.Contains(out, "- /etc/os-release:/host/etc/os-release:ro") {
		t.Errorf("docker_compose template missing host os-release mount")
	}
}

// kubernetesManifest decodes the generated multi-document manifest into its objects, by kind.
func kubernetesManifest(t *testing.T, out string) map[string][]json.RawMessage {
	t.Helper()
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(out)), 4096)
	objs := map[string][]json.RawMessage{}
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			return objs
		}
		require.NoError(t, err)
		var head struct {
			Kind string `json:"kind"`
		}
		require.NoError(t, json.Unmarshal(raw, &head))
		objs[head.Kind] = append(objs[head.Kind], raw)
	}
}

func decodeOne[T any](t *testing.T, objs map[string][]json.RawMessage, kind string) T {
	t.Helper()
	var obj T
	require.Len(t, objs[kind], 1, "want exactly one %s", kind)
	dec := json.NewDecoder(bytes.NewReader(objs[kind][0]))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&obj), "%s has a field Kubernetes does not know", kind)
	return obj
}

func TestBuildInstallKubernetes_OneAgentPerCluster(t *testing.T) {
	objs := kubernetesManifest(t, buildInstallKubernetes("grpcs://h:8443", "mnt_enr_xyz"))

	assert.Empty(t, objs["DaemonSet"], "one pod per node duplicates the cluster topology")
	secret := decodeOne[corev1.Secret](t, objs, "Secret")
	assert.Equal(t, "mnt_enr_xyz", secret.StringData["token"])

	dep := decodeOne[appsv1.Deployment](t, objs, "Deployment")
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)
	assert.Equal(t, appsv1.RecreateDeploymentStrategyType, dep.Spec.Strategy.Type)

	pod := dep.Spec.Template.Spec
	require.Len(t, pod.Containers, 1)
	c := pod.Containers[0]
	assert.Contains(t, c.Args, "--mode=agent")
	assert.Contains(t, c.Args, "--server=grpcs://h:8443")
	assert.Contains(t, c.Args, "--runtime=kubernetes")

	env := map[string]corev1.EnvVar{}
	for _, e := range c.Env {
		env[e.Name] = e
	}
	require.NotNil(t, env["MAINTENANT_ENROLLMENT_TOKEN"].ValueFrom)
	assert.Equal(t, "maintenant-agent-enrollment", env["MAINTENANT_ENROLLMENT_TOKEN"].ValueFrom.SecretKeyRef.Name)
	require.NotNil(t, env["MAINTENANT_NODE_NAME"].ValueFrom)
	assert.Equal(t, "spec.nodeName", env["MAINTENANT_NODE_NAME"].ValueFrom.FieldRef.FieldPath)
}

func TestBuildInstallKubernetes_StateSurvivesAReschedule(t *testing.T) {
	objs := kubernetesManifest(t, buildInstallKubernetes("grpcs://h:8443", "mnt_enr_xyz"))
	pvc := decodeOne[corev1.PersistentVolumeClaim](t, objs, "PersistentVolumeClaim")
	dep := decodeOne[appsv1.Deployment](t, objs, "Deployment")

	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes)
	assert.Equal(t, pvc.Namespace, dep.Namespace)

	pod := dep.Spec.Template.Spec
	volumes := map[string]corev1.Volume{}
	for _, v := range pod.Volumes {
		assert.Nil(t, v.HostPath, "volume %q pins the agent state to one node", v.Name)
		volumes[v.Name] = v
	}
	var dataMount *corev1.VolumeMount
	for i, m := range pod.Containers[0].VolumeMounts {
		if m.MountPath == "/var/lib/maintenant" {
			dataMount = &pod.Containers[0].VolumeMounts[i]
		}
	}
	require.NotNil(t, dataMount, "nothing mounted on the agent data directory")
	claim := volumes[dataMount.Name].PersistentVolumeClaim
	require.NotNil(t, claim, "the agent data directory is not backed by a claim")
	assert.Equal(t, pvc.Name, claim.ClaimName)
}

func TestBuildInstallKubernetes_Hardened(t *testing.T) {
	objs := kubernetesManifest(t, buildInstallKubernetes("grpcs://h:8443", "mnt_enr_xyz"))
	dep := decodeOne[appsv1.Deployment](t, objs, "Deployment")

	podSC := dep.Spec.Template.Spec.SecurityContext
	require.NotNil(t, podSC)
	require.NotNil(t, podSC.RunAsNonRoot)
	assert.True(t, *podSC.RunAsNonRoot)
	require.NotNil(t, podSC.RunAsUser)
	assert.Equal(t, int64(65534), *podSC.RunAsUser)
	require.NotNil(t, podSC.FSGroup)
	assert.Equal(t, int64(65534), *podSC.FSGroup)

	sc := dep.Spec.Template.Spec.Containers[0].SecurityContext
	require.NotNil(t, sc)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.Capabilities)
	assert.Equal(t, []corev1.Capability{"ALL"}, sc.Capabilities.Drop)
}

func TestBuildInstallKubernetes_GrantsWhatTheRuntimeReads(t *testing.T) {
	objs := kubernetesManifest(t, buildInstallKubernetes("grpcs://h:8443", "mnt_enr_xyz"))

	role := decodeOne[rbacv1.ClusterRole](t, objs, "ClusterRole")
	assert.Equal(t, kubernetes.ReadRules(), role.Rules)

	sa := decodeOne[corev1.ServiceAccount](t, objs, "ServiceAccount")
	binding := decodeOne[rbacv1.ClusterRoleBinding](t, objs, "ClusterRoleBinding")
	assert.Equal(t, role.Name, binding.RoleRef.Name)
	require.Len(t, binding.Subjects, 1)
	assert.Equal(t, sa.Name, binding.Subjects[0].Name)
	assert.Equal(t, sa.Namespace, binding.Subjects[0].Namespace)
}
