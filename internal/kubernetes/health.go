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

package kubernetes

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/runtime"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getHealthInfo returns health info for a workload identified by externalID.
func (r *Runtime) getHealthInfo(ctx context.Context, externalID string) (*runtime.HealthInfo, error) {
	ns, kind, name, err := parseExternalID(externalID)
	if err != nil {
		return nil, err
	}

	if kind != "" {
		return r.controllerHealth(ctx, ns, kind, name)
	}
	return r.podHealth(ctx, ns, name)
}

func (r *Runtime) podHealth(ctx context.Context, ns, podName string) (*runtime.HealthInfo, error) {
	pod, err := r.clientset.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s/%s: %w", ns, podName, err)
	}

	return mapPodHealth(pod), nil
}

func (r *Runtime) controllerHealth(ctx context.Context, ns, kind, name string) (*runtime.HealthInfo, error) {
	selector, err := r.controllerSelector(ctx, ns, kind, name)
	if err != nil {
		return nil, err
	}

	podList, err := r.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s/%s/%s: %w", ns, kind, name, err)
	}

	if len(podList.Items) == 0 {
		return &runtime.HealthInfo{
			HasHealthCheck: false,
			Status:         "none",
		}, nil
	}

	// Aggregate: all ready = healthy, any not ready = unhealthy.
	allReady := true
	hasProbes := false
	var maxRestarts int32
	var lastMessage string

	for i := range podList.Items {
		pod := &podList.Items[i]
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > maxRestarts {
				maxRestarts = cs.RestartCount
			}
			if !cs.Ready {
				allReady = false
				if cs.State.Waiting != nil && cs.State.Waiting.Message != "" {
					lastMessage = cs.State.Waiting.Reason + ": " + cs.State.Waiting.Message
				}
			}
		}
		// Check if any container has readiness probes.
		for _, c := range pod.Spec.Containers {
			if c.ReadinessProbe != nil {
				hasProbes = true
			}
		}
	}

	status := "healthy"
	if !allReady {
		status = "unhealthy"
	}

	// Check if any pod is still initializing.
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodPending {
			status = "starting"
			break
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "PodInitializing" {
				status = "starting"
				break
			}
		}
	}

	if !hasProbes {
		status = "none"
	}

	return &runtime.HealthInfo{
		HasHealthCheck: hasProbes,
		Status:         status,
		FailingStreak:  int(maxRestarts),
		LastOutput:     lastMessage,
	}, nil
}

func mapPodHealth(pod *corev1.Pod) *runtime.HealthInfo {
	hasProbes := false
	for _, c := range pod.Spec.Containers {
		if c.ReadinessProbe != nil {
			hasProbes = true
			break
		}
	}

	if !hasProbes {
		return &runtime.HealthInfo{
			HasHealthCheck: false,
			Status:         "none",
		}
	}

	allReady := true
	var maxRestarts int32
	var lastMessage string

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.RestartCount > maxRestarts {
			maxRestarts = cs.RestartCount
		}
		if !cs.Ready {
			allReady = false
			if cs.State.Waiting != nil && cs.State.Waiting.Message != "" {
				lastMessage = cs.State.Waiting.Reason + ": " + cs.State.Waiting.Message
			}
		}
	}

	status := "healthy"
	if pod.Status.Phase == corev1.PodPending {
		status = "starting"
	} else if !allReady {
		status = "unhealthy"
	}

	return &runtime.HealthInfo{
		HasHealthCheck: true,
		Status:         status,
		FailingStreak:  int(maxRestarts),
		LastOutput:     lastMessage,
	}
}
