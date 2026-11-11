// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cmodel "github.com/kolapsis/maintenant/internal/container"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8sWorkload represents a Kubernetes controller.
type K8sWorkload struct {
	ID              string // "{namespace}/{kind}/{name}"
	AgentID         string // reporting agent; empty/LocalAgent for the server's own cluster
	Name            string
	Namespace       string
	Kind            string // Deployment, StatefulSet, DaemonSet, Job
	Images          []string
	ReadyReplicas   int32
	DesiredReplicas int32
	Status          string // healthy, degraded, progressing, failed
	Conditions      []K8sCondition
	Labels          map[string]string
	Ignored         bool // maintenant.ignore annotation
	CreatedAt       time.Time
	LastTransition  time.Time
}

// K8sPod represents a single Kubernetes pod.
type K8sPod struct {
	AgentID      string // reporting agent; empty/LocalAgent for the server's own cluster
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
	Ignored      bool   // maintenant.ignore annotation
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

// K8sEventRef is a K8sEvent plus the object it concerns, so the server can store
// it per-agent and serve it back for the matching workload or pod detail view.
type K8sEventRef struct {
	K8sEvent
	InvolvedKind      string
	InvolvedNamespace string
	InvolvedName      string
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
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	targetNS := r.resolveNamespaces(namespaces)

	byNS := make(map[string][]K8sWorkload)
	for _, ns := range targetNS {
		byNS[ns] = nil
	}

	err = collectWorkloads(r, "deployments", targetNS, byNS, func() ([]appsv1.Deployment, error) {
		l, err := cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return l.Items, nil
	}, mapDeploymentWorkload)
	if err != nil {
		return nil, err
	}

	err = collectWorkloads(r, "statefulsets", targetNS, byNS, func() ([]appsv1.StatefulSet, error) {
		l, err := cs.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return l.Items, nil
	}, mapStatefulSetWorkload)
	if err != nil {
		return nil, err
	}

	err = collectWorkloads(r, "daemonsets", targetNS, byNS, func() ([]appsv1.DaemonSet, error) {
		l, err := cs.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return l.Items, nil
	}, mapDaemonSetWorkload)
	if err != nil {
		return nil, err
	}

	err = collectWorkloads(r, "jobs", targetNS, byNS, func() ([]batchv1.Job, error) {
		l, err := cs.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return l.Items, nil
	}, mapJobWorkload)
	if err != nil {
		return nil, err
	}

	return groupWorkloads(byNS), nil
}

func collectWorkloads[T any, PT interface {
	*T
	metav1.Object
}](r *Runtime, kind string, targetNS []string, byNS map[string][]K8sWorkload, list func() ([]T, error), mapFn func(PT) K8sWorkload) error {
	items, err := list()
	if err != nil {
		if !k8serrors.IsForbidden(err) {
			return fmt.Errorf("list %s: %w", kind, err)
		}
		r.logger.Warn("RBAC: forbidden to list "+kind, "error", err)
		return nil
	}
	for i := range items {
		item := PT(&items[i])
		ns := item.GetNamespace()
		if !r.nsFilter.IsAllowed(ns) {
			continue
		}
		if len(targetNS) > 0 && !containsString(targetNS, ns) {
			continue
		}
		byNS[ns] = append(byNS[ns], mapFn(item))
	}
	return nil
}

func groupWorkloads(byNS map[string][]K8sWorkload) []K8sWorkloadGroup {
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
	return groups
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
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	targetNS := r.resolveNamespaces(namespaces)

	listNS := ""
	if len(targetNS) == 1 {
		listNS = targetNS[0]
	}

	podList, err := cs.CoreV1().Pods(listNS).List(ctx, metav1.ListOptions{})
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
	cs, err := r.client()
	if err != nil {
		return nil, nil, err
	}
	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	switch kind {
	case "Deployment":
		dep, err := cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get deployment %s/%s: %w", ns, name, err)
		}
		wl := mapDeploymentWorkload(dep)
		return &wl, nil
	case "StatefulSet":
		ss, err := cs.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get statefulset %s/%s: %w", ns, name, err)
		}
		wl := mapStatefulSetWorkload(ss)
		return &wl, nil
	case "DaemonSet":
		ds, err := cs.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get daemonset %s/%s: %w", ns, name, err)
		}
		wl := mapDaemonSetWorkload(ds)
		return &wl, nil
	case "Job":
		job, err := cs.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
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
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	podList, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
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

// ListAllEvents returns recent events across all allowed namespaces, each tagged
// with the object it concerns, so the server can persist them per-agent and
// serve them back on workload/pod detail views.
func (r *Runtime) ListAllEvents(ctx context.Context) ([]K8sEventRef, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	evtList, err := cs.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list all events: %w", err)
	}

	result := make([]K8sEventRef, 0, len(evtList.Items))
	for i := range evtList.Items {
		e := &evtList.Items[i]
		if !r.nsFilter.IsAllowed(e.InvolvedObject.Namespace) {
			continue
		}
		source := e.Source.Component
		if e.Source.Host != "" {
			source += "/" + e.Source.Host
		}
		result = append(result, K8sEventRef{
			K8sEvent: K8sEvent{
				Type:      e.Type,
				Reason:    e.Reason,
				Message:   e.Message,
				Source:    source,
				FirstSeen: e.FirstTimestamp.Time,
				LastSeen:  e.LastTimestamp.Time,
				Count:     e.Count,
			},
			InvolvedKind:      e.InvolvedObject.Kind,
			InvolvedNamespace: e.InvolvedObject.Namespace,
			InvolvedName:      e.InvolvedObject.Name,
		})
	}
	return result, nil
}

func (r *Runtime) fetchEvents(ctx context.Context, ns, fieldSelector string) ([]K8sEvent, error) {
	cs, err := r.client()
	if err != nil {
		return nil, err
	}
	evtList, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
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
	wl := baseWorkload("Deployment", dep.ObjectMeta, dep.Spec.Template.Spec.Containers)
	wl.ReadyReplicas = dep.Status.ReadyReplicas
	wl.DesiredReplicas = desiredReplicas(dep.Spec.Replicas)
	wl.Conditions, wl.LastTransition = collectConditions(dep.Status.Conditions, func(c appsv1.DeploymentCondition) K8sCondition {
		return newCondition(string(c.Type), c.Status, c.Reason, c.Message, c.LastTransitionTime)
	})
	wl.Status = workloadStatus(wl.ReadyReplicas, wl.DesiredReplicas, wl.Conditions, "Progressing")
	return wl
}

func mapStatefulSetWorkload(ss *appsv1.StatefulSet) K8sWorkload {
	wl := baseWorkload("StatefulSet", ss.ObjectMeta, ss.Spec.Template.Spec.Containers)
	wl.ReadyReplicas = ss.Status.ReadyReplicas
	wl.DesiredReplicas = desiredReplicas(ss.Spec.Replicas)
	wl.Conditions, wl.LastTransition = collectConditions(ss.Status.Conditions, func(c appsv1.StatefulSetCondition) K8sCondition {
		return newCondition(string(c.Type), c.Status, c.Reason, c.Message, c.LastTransitionTime)
	})
	wl.Status = workloadStatus(wl.ReadyReplicas, wl.DesiredReplicas, wl.Conditions, "")
	return wl
}

func mapDaemonSetWorkload(ds *appsv1.DaemonSet) K8sWorkload {
	wl := baseWorkload("DaemonSet", ds.ObjectMeta, ds.Spec.Template.Spec.Containers)
	wl.ReadyReplicas = ds.Status.NumberReady
	wl.DesiredReplicas = ds.Status.DesiredNumberScheduled
	wl.Conditions, wl.LastTransition = collectConditions(ds.Status.Conditions, func(c appsv1.DaemonSetCondition) K8sCondition {
		return newCondition(string(c.Type), c.Status, c.Reason, c.Message, c.LastTransitionTime)
	})
	wl.Status = workloadStatus(wl.ReadyReplicas, wl.DesiredReplicas, wl.Conditions, "")
	return wl
}

func mapJobWorkload(job *batchv1.Job) K8sWorkload {
	wl := baseWorkload("Job", job.ObjectMeta, job.Spec.Template.Spec.Containers)
	wl.ReadyReplicas = job.Status.Active
	wl.DesiredReplicas = desiredReplicas(job.Spec.Completions)
	wl.Conditions, wl.LastTransition = collectConditions(job.Status.Conditions, func(c batchv1.JobCondition) K8sCondition {
		return newCondition(string(c.Type), c.Status, c.Reason, c.Message, c.LastTransitionTime)
	})
	wl.Status = "progressing"
	if job.Status.Succeeded >= wl.DesiredReplicas {
		wl.Status = "healthy"
	} else if job.Status.Failed > 0 && job.Status.Active == 0 {
		wl.Status = "failed"
	}
	return wl
}

// baseWorkload fills the fields every controller kind shares.
func baseWorkload(kind string, meta metav1.ObjectMeta, containers []corev1.Container) K8sWorkload {
	return K8sWorkload{
		ID:        workloadID(meta.Namespace, kind, meta.Name),
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Kind:      kind,
		Images:    containerImages(containers),
		Labels:    meta.Labels,
		Ignored:   cmodel.IgnoredByLabels(meta.Annotations),
		CreatedAt: meta.CreationTimestamp.Time,
	}
}

func newCondition(typ string, status corev1.ConditionStatus, reason, message string, at metav1.Time) K8sCondition {
	return K8sCondition{Type: typ, Status: string(status), Reason: reason, Message: message, LastTransition: at.Time}
}

// collectConditions converts controller conditions and returns them with the latest transition time.
func collectConditions[C any](conds []C, convert func(C) K8sCondition) ([]K8sCondition, time.Time) {
	out := make([]K8sCondition, 0, len(conds))
	var last time.Time
	for _, c := range conds {
		k := convert(c)
		out = append(out, k)
		if k.LastTransition.After(last) {
			last = k.LastTransition
		}
	}
	return out, last
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
		Ignored:      cmodel.IgnoredByLabels(pod.Annotations),
		CreatedAt:    pod.CreationTimestamp.Time,
	}
}

// podWorkloadRef returns the owning workload ID for a pod, the Deployment for a
// pod of one of its ReplicaSets, or empty string if the pod is standalone.
func podWorkloadRef(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		switch ref.Kind {
		case "ReplicaSet":
			if hash := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]; hash != "" && strings.HasSuffix(ref.Name, "-"+hash) {
				return workloadID(pod.Namespace, "Deployment", strings.TrimSuffix(ref.Name, "-"+hash))
			}
			return workloadID(pod.Namespace, "ReplicaSet", ref.Name)
		case "StatefulSet", "DaemonSet", "Job":
			return workloadID(pod.Namespace, ref.Kind, ref.Name)
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

// workloadID identifies a controller as namespace/Kind/name.
func workloadID(namespace, kind, name string) string {
	return namespace + "/" + kind + "/" + name
}

// desiredReplicas reads an optional replica count, which Kubernetes defaults to 1.
func desiredReplicas(n *int32) int32 {
	if n == nil {
		return 1
	}
	return *n
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
