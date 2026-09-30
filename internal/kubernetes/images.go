// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

// WorkloadImage is what the cluster reports about the image a workload runs.
type WorkloadImage struct {
	Annotations  map[string]string // the update tracking annotations
	Container    string            // the pod container that runs the workload image
	RepoDigests  []string          // "repo@sha256:..." pulled by the running pods
	DigestsKnown bool              // every running pod reports the same image; known and empty marks an image no registry served
}

// WorkloadImages returns, by external ID, the image of every workload that discovery maps to a container.
func (r *Runtime) WorkloadImages(ctx context.Context) (map[string]WorkloadImage, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}

	var pods []corev1.Pod
	podList, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	switch {
	case err == nil:
		pods = podList.Items
	case !k8serrors.IsForbidden(err):
		return nil, fmt.Errorf("list pods: %w", err)
	}

	out := make(map[string]WorkloadImage)
	add := func(ns, kind, name string, annotations map[string]string, selector *metav1.LabelSelector, spec corev1.PodSpec) {
		if r.nsFilter.IsAllowed(ns) {
			out[fmt.Sprintf("%s/%s/%s", ns, kind, name)] = workloadImage(annotations, spec, controlledPods(pods, ns, selector))
		}
	}

	deployments, err := cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil && !k8serrors.IsForbidden(err) {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	if err == nil {
		for i := range deployments.Items {
			d := &deployments.Items[i]
			add(d.Namespace, "Deployment", d.Name, d.Annotations, d.Spec.Selector, d.Spec.Template.Spec)
		}
	}

	statefulSets, err := cs.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil && !k8serrors.IsForbidden(err) {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	if err == nil {
		for i := range statefulSets.Items {
			s := &statefulSets.Items[i]
			add(s.Namespace, "StatefulSet", s.Name, s.Annotations, s.Spec.Selector, s.Spec.Template.Spec)
		}
	}

	daemonSets, err := cs.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil && !k8serrors.IsForbidden(err) {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}
	if err == nil {
		for i := range daemonSets.Items {
			d := &daemonSets.Items[i]
			add(d.Namespace, "DaemonSet", d.Name, d.Annotations, d.Spec.Selector, d.Spec.Template.Spec)
		}
	}

	for i := range pods {
		p := &pods[i]
		if r.nsFilter.IsAllowed(p.Namespace) && !hasControllerOwner(p) {
			out[p.Namespace+"/"+p.Name] = workloadImage(p.Annotations, p.Spec, []*corev1.Pod{p})
		}
	}
	return out, nil
}

// controlledPods returns the pods of namespace ns that a controller owns and selector matches.
func controlledPods(pods []corev1.Pod, ns string, selector *metav1.LabelSelector) []*corev1.Pod {
	sel, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil || sel.Empty() {
		return nil
	}
	var out []*corev1.Pod
	for i := range pods {
		p := &pods[i]
		if p.Namespace == ns && hasControllerOwner(p) && sel.Matches(labels.Set(p.Labels)) {
			out = append(out, p)
		}
	}
	return out
}

func workloadImage(annotations map[string]string, spec corev1.PodSpec, pods []*corev1.Pod) WorkloadImage {
	w := WorkloadImage{Annotations: cmodel.UpdateLabels(annotations)}
	if len(spec.Containers) == 0 {
		return w
	}
	w.Container = spec.Containers[0].Name
	w.RepoDigests, w.DigestsKnown = runningImage(pods, w.Container)
	return w
}

// runningImage returns the repo digest that container runs in the running pods; not known while none runs it or while they disagree, as during a rollout.
func runningImage(pods []*corev1.Pod, container string) ([]string, bool) {
	var ref string
	seen := false
	for _, p := range pods {
		for _, st := range p.Status.ContainerStatuses {
			if st.Name != container || st.State.Running == nil || st.ImageID == "" {
				continue
			}
			r := repoDigestOf(st.ImageID)
			if seen && r != ref {
				return nil, false
			}
			ref, seen = r, true
		}
	}
	switch {
	case !seen:
		return nil, false
	case ref == "":
		return []string{}, true
	default:
		return []string{ref}, true
	}
}

// repoDigestOf reads "repo@sha256:..." out of a container status image ID, empty for an image the node did not pull from a registry.
func repoDigestOf(imageID string) string {
	ref := strings.TrimPrefix(imageID, "docker-pullable://")
	if strings.Contains(ref, "@") {
		return ref
	}
	return ""
}
