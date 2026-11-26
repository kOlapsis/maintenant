// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kolapsis/maintenant/internal/alert"
)

func TestMapPod_ReadsTheInitContainers(t *testing.T) {
	pod := mapPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "shop"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name: "schema", RestartCount: 4,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	})
	assert.Equal(t, "CrashLoopBackOff", pod.StatusReason)
	assert.Equal(t, int32(4), pod.RestartCount)
}

func TestCheck_BarePodPendingAlertsAfterTheGracePeriod(t *testing.T) {
	h := newCheckerHarness()
	pod := K8sPod{Name: "debug", Namespace: "shop", Status: "Pending", StatusReason: "ImagePullBackOff"}

	assert.Empty(t, h.check(TopologySnapshot{Pods: []K8sPod{pod}}))
	h.advance(defaultReplicaGracePeriod)
	fired := h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	require.Len(t, fired, 1)
	assert.Equal(t, "pod_pending", fired[0].AlertType)
	assert.Equal(t, "pod", fired[0].EntityType)
	assert.Equal(t, "shop/debug", fired[0].EntityID)
	assert.Equal(t, alert.SeverityWarning, fired[0].Severity)
	assert.False(t, fired[0].IsRecover)
	assert.Empty(t, h.check(TopologySnapshot{Pods: []K8sPod{pod}}), "raised once")

	pod.Status, pod.StatusReason = "Running", ""
	recovered := h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "pod_pending", recovered[0].AlertType)
}

func TestCheck_PendingPodOfAWatchedWorkloadRaisesNothing(t *testing.T) {
	h := newCheckerHarness()
	wl := deployment(1, 1)
	pod := K8sPod{Name: "api-7d9f-x", Namespace: "shop", Status: "Pending", WorkloadRef: wl.ID}
	snap := TopologySnapshot{Workloads: []K8sWorkload{wl}, Pods: []K8sPod{pod}}

	h.check(snap)
	h.advance(defaultReplicaGracePeriod)
	assert.Empty(t, h.check(snap), "replica_health watches the pods of a Deployment")
}

func TestResume_TakesOverAPendingAlert(t *testing.T) {
	h := newCheckerHarness()
	h.c.Resume([]*alert.Alert{
		{Source: "kubernetes", AlertType: "pod_pending", EntityType: "pod", EntityID: "shop/debug", Severity: alert.SeverityWarning},
	})
	recovered := h.check(TopologySnapshot{})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "pod_pending", recovered[0].AlertType)
}

func TestCheck_PendingAlertsOnlyOnAPodThatCannotStart(t *testing.T) {
	waiting := func(reason string) corev1.ContainerStatus {
		return corev1.ContainerStatus{Name: "web", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}
	}
	pending := func(status corev1.PodStatus) K8sPod {
		status.Phase = corev1.PodPending
		return mapPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "job-x", Namespace: "batch"}, Status: status})
	}
	unscheduled := func(reason string) K8sPod {
		return pending(corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: reason}}})
	}
	cases := []struct {
		name  string
		pod   K8sPod
		alert bool
	}{
		{"init container running", pending(corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "migrate", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
			ContainerStatuses:     []corev1.ContainerStatus{waiting("PodInitializing")},
		}), false},
		{"pulling a large image", pending(corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{waiting("ContainerCreating")}}), false},
		{"waiting for its scheduling gate", unscheduled(corev1.PodReasonSchedulingGated), false},
		{"unschedulable", unscheduled(corev1.PodReasonUnschedulable), true},
		{"image cannot be pulled", pending(corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{waiting("ErrImagePull")}}), true},
		{"missing secret", pending(corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{waiting("CreateContainerConfigError")}}), true},
		{"invalid image name", pending(corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{waiting("InvalidImageName")}}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCheckerHarness()
			h.check(TopologySnapshot{Pods: []K8sPod{tc.pod}})
			h.advance(defaultReplicaGracePeriod)
			fired := h.check(TopologySnapshot{Pods: []K8sPod{tc.pod}})
			if !tc.alert {
				assert.Empty(t, fired, "a pod that is starting")
				return
			}
			require.Len(t, fired, 1)
			assert.Equal(t, "pod_pending", fired[0].AlertType)
		})
	}
}
