// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kolapsis/maintenant/internal/alert"
	cmodel "github.com/kolapsis/maintenant/internal/container"
)

func rollout(ready, desired int32, progressing corev1.ConditionStatus, reason string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"},
		Spec:       appsv1.DeploymentSpec{Replicas: &desired},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: ready,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: progressing, Reason: reason, Message: `ReplicaSet "api-5f7" has timed out progressing.`},
			},
		},
	}
}

func TestRollout_StuckPastItsDeadlineIsDegradedAndAlerts(t *testing.T) {
	dep := rollout(3, 3, corev1.ConditionFalse, "ProgressDeadlineExceeded")

	wl := mapDeploymentWorkload(dep)
	assert.Equal(t, "degraded", wl.Status)

	state, detail := deploymentState(dep)
	assert.Equal(t, cmodel.StateRunning, state, "the old replicas still serve")
	assert.Contains(t, detail, "ProgressDeadlineExceeded")

	h := newCheckerHarness()
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{wl}}))
	h.advance(defaultReplicaGracePeriod)
	fired := h.check(TopologySnapshot{Workloads: []K8sWorkload{wl}})
	require.Len(t, fired, 1)
	assert.False(t, fired[0].IsRecover)
	assert.Equal(t, alert.SeverityWarning, fired[0].Severity)
	assert.Contains(t, fired[0].Message, "progress deadline")
}

func TestRollout_StuckWithNoReadyReplicaIsExited(t *testing.T) {
	state, _ := deploymentState(rollout(0, 3, corev1.ConditionFalse, "ProgressDeadlineExceeded"))
	assert.Equal(t, cmodel.StateExited, state)
}

func TestRollout_InProgressWithinItsDeadlineRaisesNothing(t *testing.T) {
	wl := mapDeploymentWorkload(rollout(6, 8, corev1.ConditionTrue, "ReplicaSetUpdated"))
	require.Equal(t, "progressing", wl.Status)

	h := newCheckerHarness()
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{wl}}))
	h.advance(2 * defaultReplicaGracePeriod)
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{wl}}))
}

func TestRollout_InProgressKeepsAnAlertAlreadyRaised(t *testing.T) {
	h := newCheckerHarness()
	h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}})
	h.advance(defaultReplicaGracePeriod)
	require.Len(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}}), 1)

	wl := mapDeploymentWorkload(rollout(1, 3, corev1.ConditionTrue, "ReplicaSetUpdated"))
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{wl}}), "a rollout is no recovery")
}
