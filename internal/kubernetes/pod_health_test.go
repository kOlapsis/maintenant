// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

// probedPod is a bare pod whose web container has a readiness probe that cannot fail before 40 seconds.
func probedPod(phase corev1.PodPhase, ready bool, startedAgo time.Duration) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "shop"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "web", Image: "web:1",
			ReadinessProbe: &corev1.Probe{InitialDelaySeconds: 10, PeriodSeconds: 10, FailureThreshold: 3},
		}}},
		Status: corev1.PodStatus{Phase: phase},
	}
	if phase == corev1.PodRunning {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "web", Ready: ready,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now().Add(-startedAgo))}},
		}}
	}
	return pod
}

func healthEvent(t *testing.T, oldPod, newPod *corev1.Pod) string {
	t.Helper()
	for _, evt := range podUpdateEvents(oldPod, newPod) {
		if evt.Action == "health_status" {
			return evt.HealthStatus
		}
	}
	return ""
}

func TestPodUpdateEvents_APodStartingWithinItsProbeWindowIsStarting(t *testing.T) {
	pending := probedPod(corev1.PodPending, false, 0)
	started := probedPod(corev1.PodRunning, false, 5*time.Second)

	assert.Equal(t, "starting", healthEvent(t, pending, started))
	assert.Equal(t, "healthy", healthEvent(t, started, probedPod(corev1.PodRunning, true, 20*time.Second)))
	assert.Equal(t, "unhealthy", healthEvent(t, probedPod(corev1.PodRunning, true, time.Hour), probedPod(corev1.PodRunning, false, time.Hour)))
}

func TestMapBarePod_ReportsAPodNeverReadyPastItsProbeWindowAsUnhealthy(t *testing.T) {
	r := &Runtime{}
	starting := r.mapBarePod(probedPod(corev1.PodRunning, false, 5*time.Second), time.Now())
	require.NotNil(t, starting.HealthStatus)
	assert.Equal(t, cmodel.HealthStarting, *starting.HealthStatus)

	stuck := r.mapBarePod(probedPod(corev1.PodRunning, false, time.Minute), time.Now())
	require.NotNil(t, stuck.HealthStatus)
	assert.Equal(t, cmodel.HealthUnhealthy, *stuck.HealthStatus)

	assert.Nil(t, r.mapBarePod(probedPod(corev1.PodPending, false, 0), time.Now()).HealthStatus)
}
