// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"strconv"
	"time"

	cmodel "github.com/kolapsis/maintenant/internal/container"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// discoverAll lists Deployments, StatefulSets, DaemonSets, and bare pods.
func (r *Runtime) discoverAll(ctx context.Context) ([]*cmodel.Container, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var containers []*cmodel.Container
	var rbacDenied int

	// Deployments
	depList, err := cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			r.logger.Warn("RBAC: forbidden to list deployments, skipping", "error", err)
			rbacDenied++
		} else {
			return nil, fmt.Errorf("list deployments: %w", err)
		}
	} else {
		for i := range depList.Items {
			dep := &depList.Items[i]
			if !r.nsFilter.IsAllowed(dep.Namespace) {
				continue
			}
			containers = append(containers, r.mapDeployment(dep, now))
		}
	}

	// StatefulSets
	ssList, err := cs.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			r.logger.Warn("RBAC: forbidden to list statefulsets, skipping", "error", err)
			rbacDenied++
		} else {
			return nil, fmt.Errorf("list statefulsets: %w", err)
		}
	} else {
		for i := range ssList.Items {
			ss := &ssList.Items[i]
			if !r.nsFilter.IsAllowed(ss.Namespace) {
				continue
			}
			containers = append(containers, r.mapStatefulSet(ss, now))
		}
	}

	// DaemonSets
	dsList, err := cs.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			r.logger.Warn("RBAC: forbidden to list daemonsets, skipping", "error", err)
			rbacDenied++
		} else {
			return nil, fmt.Errorf("list daemonsets: %w", err)
		}
	} else {
		for i := range dsList.Items {
			ds := &dsList.Items[i]
			if !r.nsFilter.IsAllowed(ds.Namespace) {
				continue
			}
			containers = append(containers, r.mapDaemonSet(ds, now))
		}
	}

	// Bare pods (no ownerReference to a controller)
	podList, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if k8serrors.IsForbidden(err) {
			r.logger.Warn("RBAC: forbidden to list pods, skipping", "error", err)
			rbacDenied++
		} else {
			return nil, fmt.Errorf("list pods: %w", err)
		}
	} else {
		for i := range podList.Items {
			pod := &podList.Items[i]
			if !r.nsFilter.IsAllowed(pod.Namespace) {
				continue
			}
			if hasControllerOwner(pod) {
				continue // managed by a controller, already counted
			}
			containers = append(containers, r.mapBarePod(pod, now))
		}
	}

	if rbacDenied == 4 {
		r.logger.Error("RBAC: all resource types denied — no workloads will be discovered. " +
			"Ensure the ServiceAccount has a ClusterRole with get/list/watch on pods, deployments, " +
			"statefulsets, and daemonsets. See deploy/kubernetes/rbac.yaml")
	}

	return containers, nil
}

func (r *Runtime) mapDeployment(dep *appsv1.Deployment, now time.Time) *cmodel.Container {
	cm := controllerContainer("Deployment", dep.ObjectMeta, dep.Spec.Template.Spec.Containers, desiredReplicas(dep.Spec.Replicas), dep.Status.ReadyReplicas, now)
	cm.State, cm.ErrorDetail = deploymentState(dep)
	return cm
}

func (r *Runtime) mapStatefulSet(ss *appsv1.StatefulSet, now time.Time) *cmodel.Container {
	return controllerContainer("StatefulSet", ss.ObjectMeta, ss.Spec.Template.Spec.Containers, desiredReplicas(ss.Spec.Replicas), ss.Status.ReadyReplicas, now)
}

func (r *Runtime) mapDaemonSet(ds *appsv1.DaemonSet, now time.Time) *cmodel.Container {
	return controllerContainer("DaemonSet", ds.ObjectMeta, ds.Spec.Template.Spec.Containers, ds.Status.DesiredNumberScheduled, ds.Status.NumberReady, now)
}

// controllerContainer maps a controller to a container, created while none of its desired pods is ready.
func controllerContainer(kind string, meta metav1.ObjectMeta, containers []corev1.Container, desired, ready int32, now time.Time) *cmodel.Container {
	state := cmodel.StateRunning
	if ready == 0 && desired > 0 {
		state = cmodel.StateCreated
	}
	cm := &cmodel.Container{
		ExternalID:         workloadID(meta.Namespace, kind, meta.Name),
		Name:               meta.Name,
		Image:              primaryImage(containers),
		State:              state,
		OrchestrationGroup: meta.Namespace,
		OrchestrationUnit:  meta.Name,
		RuntimeType:        "kubernetes",
		ControllerKind:     kind,
		Namespace:          meta.Namespace,
		PodCount:           int(desired),
		ReadyCount:         int(ready),
		AlertSeverity:      cmodel.SeverityWarning,
		RestartThreshold:   3,
		FirstSeenAt:        meta.CreationTimestamp.Time,
		LastStateChangeAt:  now,
	}
	applyAnnotations(cm, meta.Annotations)
	return cm
}

func (r *Runtime) mapBarePod(pod *corev1.Pod, now time.Time) *cmodel.Container {
	state, errorDetail := podState(pod)
	ready := 0
	if state == cmodel.StateRunning {
		ready = 1
	}
	cm := &cmodel.Container{
		ExternalID:         fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		Name:               pod.Name,
		Image:              primaryImage(podContainers(pod)),
		State:              state,
		OrchestrationGroup: pod.Namespace,
		OrchestrationUnit:  pod.Name,
		RuntimeType:        "kubernetes",
		Namespace:          pod.Namespace,
		PodCount:           1,
		ReadyCount:         ready,
		ErrorDetail:        errorDetail,
		AlertSeverity:      cmodel.SeverityWarning,
		RestartThreshold:   3,
		FirstSeenAt:        pod.CreationTimestamp.Time,
		LastStateChangeAt:  now,
	}
	applyAnnotations(cm, pod.Annotations)
	return cm
}

func deploymentState(dep *appsv1.Deployment) (cmodel.ContainerState, string) {
	for _, cond := range dep.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing && cond.Reason == "ProgressDeadlineExceeded" {
			return cmodel.StateExited, "ProgressDeadlineExceeded: " + cond.Message
		}
	}
	replicas := desiredReplicas(dep.Spec.Replicas)
	if dep.Status.ReadyReplicas >= replicas && replicas > 0 {
		return cmodel.StateRunning, ""
	}
	if dep.Status.ReadyReplicas > 0 {
		return cmodel.StateRunning, fmt.Sprintf("partial: %d/%d ready", dep.Status.ReadyReplicas, replicas)
	}
	if replicas == 0 {
		return cmodel.StateCompleted, "scaled to 0"
	}
	return cmodel.StateCreated, ""
}

func podState(pod *corev1.Pod) (cmodel.ContainerState, string) {
	switch pod.Status.Phase {
	case corev1.PodRunning:
		// Check for CrashLoopBackOff in container statuses.
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				if cs.State.Waiting.Reason == "CrashLoopBackOff" {
					return cmodel.StateRestarting, "CrashLoopBackOff: " + cs.State.Waiting.Message
				}
				if cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "ErrImagePull" {
					return cmodel.StateCreated, cs.State.Waiting.Reason + ": " + cs.State.Waiting.Message
				}
			}
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
				return cmodel.StateExited, "OOMKilled"
			}
		}
		return cmodel.StateRunning, ""
	case corev1.PodPending:
		return cmodel.StateCreated, pendingReason(pod)
	case corev1.PodSucceeded:
		return cmodel.StateCompleted, ""
	case corev1.PodFailed:
		return cmodel.StateExited, failedReason(pod)
	default:
		return cmodel.StateCreated, ""
	}
}

func pendingReason(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason + ": " + cs.State.Waiting.Message
		}
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Status == corev1.ConditionFalse && cond.Message != "" {
			return cond.Reason + ": " + cond.Message
		}
	}
	return ""
}

func failedReason(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil {
			return cs.State.Terminated.Reason + ": " + cs.State.Terminated.Message
		}
	}
	if pod.Status.Message != "" {
		return pod.Status.Reason + ": " + pod.Status.Message
	}
	return ""
}

func hasControllerOwner(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

func primaryImage(containers []corev1.Container) string {
	if len(containers) > 0 {
		return containers[0].Image
	}
	return ""
}

func podContainers(pod *corev1.Pod) []corev1.Container {
	return pod.Spec.Containers
}

// applyAnnotations reads maintenant.* annotations from K8s workloads.
func applyAnnotations(cm *cmodel.Container, annotations map[string]string) {
	if v, ok := annotations["maintenant.ignore"]; ok && (v == "true" || v == "1") {
		cm.IsIgnored = true
	}
	if v, ok := annotations["maintenant.group"]; ok && v != "" {
		cm.CustomGroup = v
	}
	if v, ok := annotations["maintenant.alert.severity"]; ok {
		switch cmodel.AlertSeverity(v) {
		case cmodel.SeverityCritical, cmodel.SeverityWarning, cmodel.SeverityInfo:
			cm.AlertSeverity = cmodel.AlertSeverity(v)
		}
	}
	if v, ok := annotations["maintenant.alert.restart_threshold"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cm.RestartThreshold = n
		}
	}
	// Fallback display name from K8s standard labels.
	if cm.Name == "" {
		if v, ok := annotations["app.kubernetes.io/name"]; ok {
			cm.Name = v
		}
	}
}
