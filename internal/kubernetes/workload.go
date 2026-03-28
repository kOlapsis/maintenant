// Copyright 2026 Benjamin Touchard (kOlapsis)
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
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8sWorkload represents a Kubernetes controller.
type K8sWorkload struct {
	ID              string            // "{namespace}/{kind}/{name}"
	Name            string
	Namespace       string
	Kind            string // Deployment, StatefulSet, DaemonSet, Job
	Images          []string
	ReadyReplicas   int32
	DesiredReplicas int32
	Status          string // healthy, degraded, progressing, failed
	Conditions      []K8sCondition
	Labels          map[string]string
	CreatedAt       time.Time
	LastTransition  time.Time
}

// K8sPod represents a single Kubernetes pod.
type K8sPod struct {
	Name         string
	Namespace    string
	Status       string // Running, Pending, Succeeded, Failed, Unknown
	StatusReason string // CrashLoopBackOff, ImagePullBackOff, etc.
	RestartCount int32
	NodeName     string
	PodIP        string
	HostIP       string
	Containers   []K8sContainerStatus
	WorkloadRef  string // owning workload ID
	CreatedAt    time.Time
}

// K8sContainerStatus holds per-container runtime state within a pod.
type K8sContainerStatus struct {
	Name         string
	Image        string
	Ready        bool
	RestartCount int32
	State        string // running, waiting, terminated
	StateReason  string
	StartedAt    *time.Time
}

// K8sCondition represents a single Kubernetes condition.
type K8sCondition struct {
	Type           string
	Status         string // True, False, Unknown
	Reason         string
	Message        string
	LastTransition time.Time
}

// K8sWorkloadGroup groups workloads by namespace.
type K8sWorkloadGroup struct {
	Namespace string
	Workloads []K8sWorkload
}

// K8sEvent is a summarised Kubernetes event (v1.Event).
type K8sEvent struct {
	Type      string
	Reason    string
	Message   string
	Source    string
	FirstSeen time.Time
	LastSeen  time.Time
	Count     int32
}

// PodFilters are optional filters for ListPods.
type PodFilters struct {
	Workload string // owning workload ID prefix
	Node     string // node name
	Status   string // Running, Pending, etc.
}

// ListWorkloads returns workloads grouped by namespace. When namespaces is
// non-empty only those namespaces are queried; otherwise all allowed
// namespaces are included.
func (r *Runtime) ListWorkloads(ctx context.Context, namespaces []string) ([]K8sWorkloadGroup, error) {
	targetNS := r.resolveNamespaces(namespaces)

	// Collect per-namespace workloads.
	byNS := make(map[string][]K8sWorkload)
	for _, ns := range targetNS {
		byNS[ns] = nil
	}

	// Deployments.
	depList, err := r.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if !k8serrors.IsForbidden(err) {
			return nil, fmt.Errorf("list deployments: %w", err)
		}
		r.logger.Warn("RBAC: forbidden to list deployments", "error", err)
	} else {
		for i := range depList.Items {
			dep := &depList.Items[i]
			if !r.nsFilter.IsAllowed(dep.Namespace) {
				continue
			}
			if len(targetNS) > 0 && !containsString(targetNS, dep.Namespace) {
				continue
			}
			byNS[dep.Namespace] = append(byNS[dep.Namespace], mapDeploymentWorkload(dep))
		}
	}

	// StatefulSets.
	ssList, err := r.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if !k8serrors.IsForbidden(err) {
			return nil, fmt.Errorf("list statefulsets: %w", err)
		}
		r.logger.Warn("RBAC: forbidden to list statefulsets", "error", err)
	} else {
		for i := range ssList.Items {
			ss := &ssList.Items[i]
			if !r.nsFilter.IsAllowed(ss.Namespace) {
				continue
			}
			if len(targetNS) > 0 && !containsString(targetNS, ss.Namespace) {
				continue
			}
			byNS[ss.Namespace] = append(byNS[ss.Namespace], mapStatefulSetWorkload(ss))
		}
	}

	// DaemonSets.
	dsList, err := r.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if !k8serrors.IsForbidden(err) {
			return nil, fmt.Errorf("list daemonsets: %w", err)
		}
		r.logger.Warn("RBAC: forbidden to list daemonsets", "error", err)
	} else {
		for i := range dsList.Items {
			ds := &dsList.Items[i]
			if !r.nsFilter.IsAllowed(ds.Namespace) {
				continue
			}
			if len(targetNS) > 0 && !containsString(targetNS, ds.Namespace) {
				continue
			}
			byNS[ds.Namespace] = append(byNS[ds.Namespace], mapDaemonSetWorkload(ds))
		}
	}

	// Jobs.
	jobList, err := r.clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if !k8serrors.IsForbidden(err) {
			return nil, fmt.Errorf("list jobs: %w", err)
		}
		r.logger.Warn("RBAC: forbidden to list jobs", "error", err)
	} else {
		for i := range jobList.Items {
			job := &jobList.Items[i]
			if !r.nsFilter.IsAllowed(job.Namespace) {
				continue
			}
			if len(targetNS) > 0 && !containsString(targetNS, job.Namespace) {
				continue
			}
			byNS[job.Namespace] = append(byNS[job.Namespace], mapJobWorkload(job))
		}
	}

	// Build sorted groups, skipping empty namespaces.
	var groups []K8sWorkloadGroup
	var nsKeys []string
	for ns := range byNS {
		nsKeys = append(nsKeys, ns)
	}
	sort.Strings(nsKeys)

	for _, ns := range nsKeys {
		wls := byNS[ns]
		if len(wls) == 0 {
			continue
		}
		sort.Slice(wls, func(i, j int) bool {
			if wls[i].Kind != wls[j].Kind {
				return wls[i].Kind < wls[j].Kind
			}
			return wls[i].Name < wls[j].Name
		})
		groups = append(groups, K8sWorkloadGroup{
			Namespace: ns,
			Workloads: wls,
		})
	}

	return groups, nil
}

// GetWorkload returns the detail for a single workload, its pods, and recent events.
// id format: "namespace/Kind/name".
func (r *Runtime) GetWorkload(ctx context.Context, id string) (*K8sWorkload, []K8sPod, []K8sEvent, error) {
	ns, kind, name, err := parseExternalID(id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse workload id: %w", err)
	}
	if kind == "" {
		return nil, nil, nil, fmt.Errorf("workload id must contain kind: %q", id)
	}

	wl, err := r.fetchWorkload(ctx, ns, kind, name)
	if err != nil {
		return nil, nil, nil, err
	}

	selector, err := r.controllerSelector(ctx, ns, kind, name)
	if err != nil {
		return wl, nil, nil, nil //nolint:nilerr // best-effort pod listing
	}

	pods, err := r.listPodsForSelector(ctx, ns, selector, id)
	if err != nil {
		return wl, nil, nil, fmt.Errorf("list pods: %w", err)
	}

	events, err := r.listWorkloadEvents(ctx, ns, kind, name)
	if err != nil {
		r.logger.Warn("failed to list workload events", "workload", id, "error", err)
		events = nil
	}

	return wl, pods, events, nil
}

// ListPods returns a flat pod list optionally filtered by workload, node, and status.
func (r *Runtime) ListPods(ctx context.Context, namespaces []string, filters PodFilters) ([]K8sPod, error) {
	targetNS := r.resolveNamespaces(namespaces)

	listNS := ""
	if len(targetNS) == 1 {
		listNS = targetNS[0]
	}

	podList, err := r.clientset.CoreV1().Pods(listNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var result []K8sPod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !r.nsFilter.IsAllowed(pod.Namespace) {
			continue
		}
		if len(targetNS) > 1 && !containsString(targetNS, pod.Namespace) {
			continue
		}
		mapped := mapPod(pod)

		if filters.Node != "" && mapped.NodeName != filters.Node {
			continue
		}
		if filters.Status != "" && !strings.EqualFold(mapped.Status, filters.Status) {
			continue
		}
		if filters.Workload != "" && !strings.HasPrefix(mapped.WorkloadRef, filters.Workload) {
			continue
		}
		result = append(result, mapped)
	}

	return result, nil
}

// GetPodDetail returns details for a single pod plus recent events.
func (r *Runtime) GetPodDetail(ctx context.Context, namespace, name string) (*K8sPod, []K8sEvent, error) {
	pod, err := r.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get pod %s/%s: %w", namespace, name, err)
	}

	mapped := mapPod(pod)

	events, err := r.listPodEvents(ctx, namespace, name)
	if err != nil {
		r.logger.Warn("failed to list pod events", "pod", namespace+"/"+name, "error", err)
		events = nil
	}

	return &mapped, events, nil
}

// --- internal helpers ---

func (r *Runtime) fetchWorkload(ctx context.Context, ns, kind, name string) (*K8sWorkload, error) {
	switch kind {
	case "Deployment":
		dep, err := r.clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get deployment %s/%s: %w", ns, name, err)
		}
		wl := mapDeploymentWorkload(dep)
		return &wl, nil
	case "StatefulSet":
		ss, err := r.clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get statefulset %s/%s: %w", ns, name, err)
		}
		wl := mapStatefulSetWorkload(ss)
		return &wl, nil
	case "DaemonSet":
		ds, err := r.clientset.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get daemonset %s/%s: %w", ns, name, err)
		}
		wl := mapDaemonSetWorkload(ds)
		return &wl, nil
	case "Job":
		job, err := r.clientset.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get job %s/%s: %w", ns, name, err)
		}
		wl := mapJobWorkload(job)
		return &wl, nil
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}

func (r *Runtime) listPodsForSelector(ctx context.Context, ns, selector, workloadID string) ([]K8sPod, error) {
	podList, err := r.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods (selector=%s): %w", selector, err)
	}

	result := make([]K8sPod, 0, len(podList.Items))
	for i := range podList.Items {
		p := mapPod(&podList.Items[i])
		p.WorkloadRef = workloadID
		result = append(result, p)
	}
	return result, nil
}

func (r *Runtime) listWorkloadEvents(ctx context.Context, ns, kind, name string) ([]K8sEvent, error) {
	fieldSelector := fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s,involvedObject.namespace=%s", kind, name, ns)
	return r.fetchEvents(ctx, ns, fieldSelector)
}

func (r *Runtime) listPodEvents(ctx context.Context, ns, name string) ([]K8sEvent, error) {
	fieldSelector := fmt.Sprintf("involvedObject.kind=Pod,involvedObject.name=%s,involvedObject.namespace=%s", name, ns)
	return r.fetchEvents(ctx, ns, fieldSelector)
}

func (r *Runtime) fetchEvents(ctx context.Context, ns, fieldSelector string) ([]K8sEvent, error) {
	evtList, err := r.clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	result := make([]K8sEvent, 0, len(evtList.Items))
	for i := range evtList.Items {
		e := &evtList.Items[i]
		source := e.Source.Component
		if e.Source.Host != "" {
			source += "/" + e.Source.Host
		}
		result = append(result, K8sEvent{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Source:    source,
			FirstSeen: e.FirstTimestamp.Time,
			LastSeen:  e.LastTimestamp.Time,
			Count:     e.Count,
		})
	}

	// Sort newest first.
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen.After(result[j].LastSeen)
	})

	return result, nil
}

// resolveNamespaces returns the requested namespaces after applying nsFilter.
// When namespaces is empty, returns nil (meaning "all allowed").
func (r *Runtime) resolveNamespaces(namespaces []string) []string {
	if len(namespaces) == 0 {
		return nil
	}
	var allowed []string
	for _, ns := range namespaces {
		if r.nsFilter.IsAllowed(ns) {
			allowed = append(allowed, ns)
		}
	}
	return allowed
}

// --- mapping functions ---

func mapDeploymentWorkload(dep *appsv1.Deployment) K8sWorkload {
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	conditions := make([]K8sCondition, 0, len(dep.Status.Conditions))
	var lastTransition time.Time
	for _, c := range dep.Status.Conditions {
		conditions = append(conditions, K8sCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
		if c.LastTransitionTime.Time.After(lastTransition) {
			lastTransition = c.LastTransitionTime.Time
		}
	}
	return K8sWorkload{
		ID:              fmt.Sprintf("%s/Deployment/%s", dep.Namespace, dep.Name),
		Name:            dep.Name,
		Namespace:       dep.Namespace,
		Kind:            "Deployment",
		Images:          containerImages(dep.Spec.Template.Spec.Containers),
		ReadyReplicas:   dep.Status.ReadyReplicas,
		DesiredReplicas: desired,
		Status:          workloadStatus(dep.Status.ReadyReplicas, desired, conditions, "Progressing"),
		Conditions:      conditions,
		Labels:          dep.Labels,
		CreatedAt:       dep.CreationTimestamp.Time,
		LastTransition:  lastTransition,
	}
}

func mapStatefulSetWorkload(ss *appsv1.StatefulSet) K8sWorkload {
	desired := int32(1)
	if ss.Spec.Replicas != nil {
		desired = *ss.Spec.Replicas
	}
	conditions := make([]K8sCondition, 0, len(ss.Status.Conditions))
	var lastTransition time.Time
	for _, c := range ss.Status.Conditions {
		conditions = append(conditions, K8sCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
		if c.LastTransitionTime.Time.After(lastTransition) {
			lastTransition = c.LastTransitionTime.Time
		}
	}
	return K8sWorkload{
		ID:              fmt.Sprintf("%s/StatefulSet/%s", ss.Namespace, ss.Name),
		Name:            ss.Name,
		Namespace:       ss.Namespace,
		Kind:            "StatefulSet",
		Images:          containerImages(ss.Spec.Template.Spec.Containers),
		ReadyReplicas:   ss.Status.ReadyReplicas,
		DesiredReplicas: desired,
		Status:          workloadStatus(ss.Status.ReadyReplicas, desired, conditions, ""),
		Conditions:      conditions,
		Labels:          ss.Labels,
		CreatedAt:       ss.CreationTimestamp.Time,
		LastTransition:  lastTransition,
	}
}

func mapDaemonSetWorkload(ds *appsv1.DaemonSet) K8sWorkload {
	desired := ds.Status.DesiredNumberScheduled
	conditions := make([]K8sCondition, 0, len(ds.Status.Conditions))
	var lastTransition time.Time
	for _, c := range ds.Status.Conditions {
		conditions = append(conditions, K8sCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
		if c.LastTransitionTime.Time.After(lastTransition) {
			lastTransition = c.LastTransitionTime.Time
		}
	}
	return K8sWorkload{
		ID:              fmt.Sprintf("%s/DaemonSet/%s", ds.Namespace, ds.Name),
		Name:            ds.Name,
		Namespace:       ds.Namespace,
		Kind:            "DaemonSet",
		Images:          containerImages(ds.Spec.Template.Spec.Containers),
		ReadyReplicas:   ds.Status.NumberReady,
		DesiredReplicas: desired,
		Status:          workloadStatus(ds.Status.NumberReady, desired, conditions, ""),
		Conditions:      conditions,
		Labels:          ds.Labels,
		CreatedAt:       ds.CreationTimestamp.Time,
		LastTransition:  lastTransition,
	}
}

func mapJobWorkload(job *batchv1.Job) K8sWorkload {
	desired := int32(1)
	if job.Spec.Completions != nil {
		desired = *job.Spec.Completions
	}
	conditions := make([]K8sCondition, 0, len(job.Status.Conditions))
	var lastTransition time.Time
	for _, c := range job.Status.Conditions {
		conditions = append(conditions, K8sCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
		if c.LastTransitionTime.Time.After(lastTransition) {
			lastTransition = c.LastTransitionTime.Time
		}
	}

	status := "progressing"
	if job.Status.Succeeded >= desired {
		status = "healthy"
	} else if job.Status.Failed > 0 && job.Status.Active == 0 {
		status = "failed"
	}

	return K8sWorkload{
		ID:              fmt.Sprintf("%s/Job/%s", job.Namespace, job.Name),
		Name:            job.Name,
		Namespace:       job.Namespace,
		Kind:            "Job",
		Images:          containerImages(job.Spec.Template.Spec.Containers),
		ReadyReplicas:   job.Status.Active,
		DesiredReplicas: desired,
		Status:          status,
		Conditions:      conditions,
		Labels:          job.Labels,
		CreatedAt:       job.CreationTimestamp.Time,
		LastTransition:  lastTransition,
	}
}

func mapPod(pod *corev1.Pod) K8sPod {
	status := string(pod.Status.Phase)
	statusReason := ""

	var totalRestarts int32
	containerStatuses := make([]K8sContainerStatus, 0, len(pod.Status.ContainerStatuses))

	for _, cs := range pod.Status.ContainerStatuses {
		totalRestarts += cs.RestartCount

		state := "waiting"
		stateReason := ""
		var startedAt *time.Time

		switch {
		case cs.State.Running != nil:
			state = "running"
			t := cs.State.Running.StartedAt.Time
			startedAt = &t
		case cs.State.Waiting != nil:
			state = "waiting"
			stateReason = cs.State.Waiting.Reason
			if stateReason == "CrashLoopBackOff" || stateReason == "ImagePullBackOff" || stateReason == "ErrImagePull" {
				statusReason = stateReason
			}
		case cs.State.Terminated != nil:
			state = "terminated"
			stateReason = cs.State.Terminated.Reason
		}

		// Look up the image from spec.
		image := cs.Image
		for _, c := range pod.Spec.Containers {
			if c.Name == cs.Name {
				image = c.Image
				break
			}
		}

		containerStatuses = append(containerStatuses, K8sContainerStatus{
			Name:         cs.Name,
			Image:        image,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			State:        state,
			StateReason:  stateReason,
			StartedAt:    startedAt,
		})
	}

	workloadRef := podWorkloadRef(pod)

	return K8sPod{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		Status:       status,
		StatusReason: statusReason,
		RestartCount: totalRestarts,
		NodeName:     pod.Spec.NodeName,
		PodIP:        pod.Status.PodIP,
		HostIP:       pod.Status.HostIP,
		Containers:   containerStatuses,
		WorkloadRef:  workloadRef,
		CreatedAt:    pod.CreationTimestamp.Time,
	}
}

// podWorkloadRef returns the owning workload ID for a pod, or empty string if
// the pod is standalone.
func podWorkloadRef(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		switch ref.Kind {
		case "ReplicaSet":
			// ReplicaSets are owned by Deployments; resolve up one level if
			// possible, but we'd need an extra API call. Use namespace/ReplicaSet/name
			// as a fallback — callers can enrich if needed.
			return fmt.Sprintf("%s/ReplicaSet/%s", pod.Namespace, ref.Name)
		case "StatefulSet", "DaemonSet", "Job":
			return fmt.Sprintf("%s/%s/%s", pod.Namespace, ref.Kind, ref.Name)
		}
	}
	return ""
}

// workloadStatus derives a status string from replica counts and conditions.
func workloadStatus(ready, desired int32, conditions []K8sCondition, progressingType string) string {
	// Check for Progressing condition.
	if progressingType != "" {
		for _, c := range conditions {
			if c.Type == progressingType && c.Status == "True" && c.Reason == "ReplicaSetUpdated" {
				return "progressing"
			}
		}
	}

	if desired == 0 {
		return "healthy" // scaled to zero is intentional
	}
	if ready >= desired {
		return "healthy"
	}
	if ready == 0 {
		return "failed"
	}
	return "degraded"
}

func containerImages(containers []corev1.Container) []string {
	images := make([]string, 0, len(containers))
	seen := make(map[string]bool, len(containers))
	for _, c := range containers {
		if c.Image != "" && !seen[c.Image] {
			images = append(images, c.Image)
			seen[c.Image] = true
		}
	}
	return images
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
