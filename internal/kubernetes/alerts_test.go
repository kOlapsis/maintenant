// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

type checkerHarness struct {
	c      *K8sAlertChecker
	clock  time.Time
	alerts []alert.Event
	events []string
}

func newCheckerHarness() *checkerHarness {
	h := &checkerHarness{clock: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	h.c = NewK8sAlertChecker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.c.now = func() time.Time { return h.clock }
	h.c.SetAlertCallback(func(evt alert.Event) { h.alerts = append(h.alerts, evt) })
	h.c.SetEventCallback(func(eventType string, _ any) { h.events = append(h.events, eventType) })
	return h
}

func (h *checkerHarness) check(snap TopologySnapshot) []alert.Event {
	h.alerts = nil
	h.c.Check(snap)
	return h.alerts
}

func (h *checkerHarness) advance(d time.Duration) { h.clock = h.clock.Add(d) }

func deployment(ready, desired int32) K8sWorkload {
	return K8sWorkload{ID: "shop/Deployment/api", Name: "api", Namespace: "shop", Kind: "Deployment", ReadyReplicas: ready, DesiredReplicas: desired}
}

func readyNode(name string) K8sNode {
	return K8sNode{Name: name, Status: "ready"}
}

func TestCheck_WorkloadUnderReplicatedAlertsOnceAndResolves(t *testing.T) {
	h := newCheckerHarness()

	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}}), "the grace period comes first")

	h.advance(defaultReplicaGracePeriod)
	fired := h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}})
	require.Len(t, fired, 1)
	assert.Equal(t, "kubernetes", fired[0].Source)
	assert.Equal(t, "replica_health", fired[0].AlertType)
	assert.Equal(t, "workload", fired[0].EntityType)
	assert.Equal(t, "shop/Deployment/api", fired[0].EntityID)
	assert.Equal(t, alert.SeverityWarning, fired[0].Severity)
	assert.False(t, fired[0].IsRecover)
	assert.Contains(t, h.events, event.KubernetesWorkloadChanged)

	h.advance(30 * time.Second)
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}}), "an alert already raised is not raised again")

	escalated := h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(0, 3)}})
	require.Len(t, escalated, 1)
	assert.Equal(t, alert.SeverityCritical, escalated[0].Severity)

	recovered := h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(3, 3)}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "shop/Deployment/api", recovered[0].EntityID)
}

func TestCheck_TwoWorkloadsKeepTheirOwnAlert(t *testing.T) {
	h := newCheckerHarness()
	other := K8sWorkload{ID: "shop/StatefulSet/db", Name: "db", Namespace: "shop", Kind: "StatefulSet", ReadyReplicas: 1, DesiredReplicas: 2}
	snap := TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3), other}}

	h.check(snap)
	h.advance(defaultReplicaGracePeriod)
	fired := h.check(snap)
	require.Len(t, fired, 2)
	assert.ElementsMatch(t, []string{"shop/Deployment/api", "shop/StatefulSet/db"}, []string{fired[0].EntityID, fired[1].EntityID})
}

func TestCheck_WorkloadGoneResolvesItsAlert(t *testing.T) {
	h := newCheckerHarness()
	h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(0, 3)}})
	h.advance(defaultReplicaGracePeriod)
	require.Len(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(0, 3)}}), 1)

	recovered := h.check(TopologySnapshot{})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "shop/Deployment/api", recovered[0].EntityID)
}

func TestCheck_JobsHaveNoReplicaAlert(t *testing.T) {
	h := newCheckerHarness()
	done := K8sWorkload{ID: "batch/Job/report", Name: "report", Namespace: "batch", Kind: "Job", ReadyReplicas: 0, DesiredReplicas: 1}
	h.check(TopologySnapshot{Workloads: []K8sWorkload{done}})
	h.advance(defaultReplicaGracePeriod)
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{done}}))
}

func TestCheck_IgnoredWorkloadRaisesNothing(t *testing.T) {
	h := newCheckerHarness()
	ignored := deployment(0, 3)
	ignored.Ignored = true
	pod := K8sPod{Name: "api-7d9f-x", Namespace: "shop", StatusReason: "CrashLoopBackOff", RestartCount: 9, WorkloadRef: "shop/Deployment/api"}
	snap := TopologySnapshot{Workloads: []K8sWorkload{ignored}, Pods: []K8sPod{pod}}

	assert.Empty(t, h.check(snap))
	h.advance(defaultReplicaGracePeriod)
	assert.Empty(t, h.check(snap), "neither the workload nor its pods alert")
}

func TestCheck_WorkloadIgnoredWhileAlertedResolves(t *testing.T) {
	h := newCheckerHarness()
	h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(0, 3)}})
	h.advance(defaultReplicaGracePeriod)
	require.Len(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(0, 3)}}), 1)

	ignored := deployment(0, 3)
	ignored.Ignored = true
	recovered := h.check(TopologySnapshot{Workloads: []K8sWorkload{ignored}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
}

func TestCheck_IgnoredBarePodRaisesNothing(t *testing.T) {
	h := newCheckerHarness()
	pod := K8sPod{Name: "debug", Namespace: "shop", StatusReason: "CrashLoopBackOff", Ignored: true}
	assert.Empty(t, h.check(TopologySnapshot{Pods: []K8sPod{pod}}))
}

func TestCheck_CrashLoopBackOffPod(t *testing.T) {
	h := newCheckerHarness()
	pod := K8sPod{Name: "api-7d9f-x", Namespace: "shop", StatusReason: "CrashLoopBackOff", RestartCount: 4}

	fired := h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	require.Len(t, fired, 1)
	assert.Equal(t, "crash_loop", fired[0].AlertType)
	assert.Equal(t, "pod", fired[0].EntityType)
	assert.Equal(t, "shop/api-7d9f-x", fired[0].EntityID)
	assert.Equal(t, alert.SeverityCritical, fired[0].Severity)
	assert.Contains(t, h.events, event.KubernetesPodChanged)

	assert.Empty(t, h.check(TopologySnapshot{Pods: []K8sPod{pod}}))

	recovered := h.check(TopologySnapshot{})
	require.Len(t, recovered, 1, "a pod replaced while crash-looping resolves its alert")
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "shop/api-7d9f-x", recovered[0].EntityID)
}

func TestCheck_FastRestartsStayAlertedUntilTheWindowPasses(t *testing.T) {
	h := newCheckerHarness()
	pod := K8sPod{Name: "worker-0", Namespace: "jobs", Status: "Running"}

	h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	pod.RestartCount = 3
	h.advance(30 * time.Second)
	fired := h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	require.Len(t, fired, 1, "three restarts between two checks count as three")
	assert.False(t, fired[0].IsRecover)

	h.advance(30 * time.Second)
	assert.Empty(t, h.check(TopologySnapshot{Pods: []K8sPod{pod}}), "a running pod still inside the window stays alerted")

	h.advance(defaultCrashLoopWindow)
	recovered := h.check(TopologySnapshot{Pods: []K8sPod{pod}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
}

func TestCheck_NodeConditionsShareOneAlert(t *testing.T) {
	h := newCheckerHarness()
	pressured := K8sNode{Name: "worker-1", Status: "ready", Conditions: []K8sCondition{
		{Type: "MemoryPressure", Status: "True"},
		{Type: "DiskPressure", Status: "True"},
	}}

	fired := h.check(TopologySnapshot{Nodes: []K8sNode{pressured}})
	require.Len(t, fired, 1)
	assert.Equal(t, "node_condition", fired[0].AlertType)
	assert.Equal(t, "node", fired[0].EntityType)
	assert.Equal(t, "worker-1", fired[0].EntityID)
	assert.Equal(t, alert.SeverityWarning, fired[0].Severity)
	assert.Contains(t, h.events, event.KubernetesNodeChanged)

	notReady := pressured
	notReady.Status = "unknown"
	escalated := h.check(TopologySnapshot{Nodes: []K8sNode{notReady}})
	require.Len(t, escalated, 1)
	assert.Equal(t, alert.SeverityCritical, escalated[0].Severity)

	assert.Empty(t, h.check(TopologySnapshot{}), "nodes that could not be listed resolve nothing")

	recovered := h.check(TopologySnapshot{Nodes: []K8sNode{readyNode("worker-1")}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "worker-1", recovered[0].EntityID)
}

func TestCheck_NodeRemovedResolvesItsAlert(t *testing.T) {
	h := newCheckerHarness()
	require.Len(t, h.check(TopologySnapshot{Nodes: []K8sNode{{Name: "worker-2", Status: "not-ready"}, readyNode("cp-1")}}), 1)

	recovered := h.check(TopologySnapshot{Nodes: []K8sNode{readyNode("cp-1")}})
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
	assert.Equal(t, "worker-2", recovered[0].EntityID)
}

func TestResume_ResolvesAlertsLeftByThePreviousRun(t *testing.T) {
	h := newCheckerHarness()
	h.c.Resume([]*alert.Alert{
		{Source: "kubernetes", AlertType: "replica_health", EntityType: "workload", EntityID: "shop/Deployment/api", Severity: alert.SeverityWarning},
		{Source: "kubernetes", AlertType: "crash_loop", EntityType: "pod", EntityID: "shop/api-old", Severity: alert.SeverityCritical},
		{Source: "kubernetes", AlertType: "node_condition", EntityType: "node", EntityID: "worker-1", Severity: alert.SeverityCritical},
		{Source: "container", AlertType: "restart_loop", EntityType: "container", EntityID: "c1"},
	})

	recovered := h.check(TopologySnapshot{
		Workloads: []K8sWorkload{deployment(3, 3)},
		Nodes:     []K8sNode{readyNode("worker-1")},
	})
	require.Len(t, recovered, 3)
	var ids []string
	for _, evt := range recovered {
		assert.True(t, evt.IsRecover)
		ids = append(ids, evt.EntityID)
	}
	assert.ElementsMatch(t, []string{"shop/Deployment/api", "shop/api-old", "worker-1"}, ids)
}

func TestResume_StillFailingRaisesNoDuplicate(t *testing.T) {
	h := newCheckerHarness()
	h.c.Resume([]*alert.Alert{
		{Source: "kubernetes", AlertType: "replica_health", EntityType: "workload", EntityID: "shop/Deployment/api", Severity: alert.SeverityWarning},
	})
	assert.Empty(t, h.check(TopologySnapshot{Workloads: []K8sWorkload{deployment(1, 3)}}))
}

func TestPodWorkloadRef_ResolvesTheDeployment(t *testing.T) {
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "api-7d9f8c6b5-x2k4p",
		Namespace: "shop",
		Labels:    map[string]string{"pod-template-hash": "7d9f8c6b5"},
		OwnerReferences: []metav1.OwnerReference{
			{Kind: "ReplicaSet", Name: "api-7d9f8c6b5", Controller: &controller},
		},
	}}
	assert.Equal(t, "shop/Deployment/api", podWorkloadRef(pod))

	pod.Labels = nil
	assert.Equal(t, "shop/ReplicaSet/api-7d9f8c6b5", podWorkloadRef(pod), "a ReplicaSet no Deployment owns stays as is")
}

func TestMapping_ReadsTheIgnoreAnnotation(t *testing.T) {
	ignore := map[string]string{"maintenant.ignore": "true"}
	meta := metav1.ObjectMeta{Name: "api", Namespace: "shop", Annotations: ignore}

	assert.True(t, mapDeploymentWorkload(&appsv1.Deployment{ObjectMeta: meta}).Ignored)
	assert.True(t, mapStatefulSetWorkload(&appsv1.StatefulSet{ObjectMeta: meta}).Ignored)
	assert.True(t, mapDaemonSetWorkload(&appsv1.DaemonSet{ObjectMeta: meta}).Ignored)
	assert.True(t, mapJobWorkload(&batchv1.Job{ObjectMeta: meta}).Ignored)
	assert.True(t, mapPod(&corev1.Pod{ObjectMeta: meta}).Ignored)
	assert.False(t, mapDeploymentWorkload(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web"}}).Ignored)
}
