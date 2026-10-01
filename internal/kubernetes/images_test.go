// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func apiDeployment(ns string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: ns,
			Annotations: map[string]string{
				"maintenant.update.track":                          "patch",
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "server", Image: "ghcr.io/acme/api:stable"},
				{Name: "proxy", Image: "envoyproxy/envoy:v1"},
			}}},
		},
	}
}

func apiPod(ns, name, imageID string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns, Labels: map[string]string{"app": "api"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-7d9f", Controller: boolPtr(true)}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
			{Name: "proxy", ImageID: "docker.io/envoyproxy/envoy@sha256:envoy", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			{Name: "server", ImageID: imageID, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
		}},
	}
}

func workloadImages(t *testing.T, objects ...any) map[string]WorkloadImage {
	t.Helper()
	cs := fake.NewClientset()
	for _, o := range objects {
		switch v := o.(type) {
		case *appsv1.Deployment:
			_, err := cs.AppsV1().Deployments(v.Namespace).Create(context.Background(), v, metav1.CreateOptions{})
			require.NoError(t, err)
		case *corev1.Pod:
			_, err := cs.CoreV1().Pods(v.Namespace).Create(context.Background(), v, metav1.CreateOptions{})
			require.NoError(t, err)
		}
	}
	got, err := withClient(newTestRuntime(t), cs).WorkloadImages(context.Background())
	require.NoError(t, err)
	return got
}

func TestWorkloadImages_DeploymentRunningOneDigest(t *testing.T) {
	got := workloadImages(t,
		apiDeployment("prod"),
		apiPod("prod", "api-7d9f-a", "ghcr.io/acme/api@sha256:running"),
		apiPod("prod", "api-7d9f-b", "docker-pullable://ghcr.io/acme/api@sha256:running"),
		apiPod("staging", "api-7d9f-c", "ghcr.io/acme/api@sha256:elsewhere"),
	)

	w, ok := got["prod/Deployment/api"]
	require.True(t, ok)
	assert.Equal(t, map[string]string{"maintenant.update.track": "patch"}, w.Annotations)
	assert.Equal(t, "server", w.Container)
	assert.True(t, w.DigestsKnown)
	assert.Equal(t, []string{"ghcr.io/acme/api@sha256:running"}, w.RepoDigests)
}

// Pods that disagree are mid-rollout: which digest the workload runs is not settled yet.
func TestWorkloadImages_PodsOnDifferentDigestsAreNotKnown(t *testing.T) {
	got := workloadImages(t,
		apiDeployment("prod"),
		apiPod("prod", "api-old", "ghcr.io/acme/api@sha256:old"),
		apiPod("prod", "api-new", "ghcr.io/acme/api@sha256:new"),
	)

	w := got["prod/Deployment/api"]
	assert.False(t, w.DigestsKnown)
	assert.Empty(t, w.RepoDigests)
	assert.Equal(t, "server", w.Container)
}

func TestWorkloadImages_ImageNoRegistryServed(t *testing.T) {
	got := workloadImages(t,
		apiDeployment("prod"),
		apiPod("prod", "api-a", "sha256:4b6e1f"),
	)

	w := got["prod/Deployment/api"]
	assert.True(t, w.DigestsKnown)
	assert.Empty(t, w.RepoDigests, "an image imported on the node has no registry digest")
}

func TestWorkloadImages_BarePodAndNoRunningPod(t *testing.T) {
	bare := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "prod", Annotations: map[string]string{"maintenant.update.enabled": "false"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "shell", Image: "busybox:1.36"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
			{Name: "shell", ImageID: "docker.io/library/busybox@sha256:bb", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
		}},
	}
	got := workloadImages(t, apiDeployment("prod"), bare)

	assert.False(t, got["prod/Deployment/api"].DigestsKnown, "no pod runs the workload yet")
	pod := got["prod/debug"]
	assert.Equal(t, "false", pod.Annotations["maintenant.update.enabled"])
	assert.Equal(t, "shell", pod.Container)
	assert.Equal(t, []string{"docker.io/library/busybox@sha256:bb"}, pod.RepoDigests)
}

func TestWorkloadImages_NamespaceFilter(t *testing.T) {
	cs := fake.NewClientset(apiDeployment("kube-system"), apiDeployment("prod"))
	r := withClient(newTestRuntime(t), cs)
	r.nsFilter = NewNamespaceFilter("", "kube-system")

	got, err := r.WorkloadImages(context.Background())
	require.NoError(t, err)
	assert.Contains(t, got, "prod/Deployment/api")
	assert.NotContains(t, got, "kube-system/Deployment/api")
}
