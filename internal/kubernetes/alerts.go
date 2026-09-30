// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/event"
)

const (
	alertSource = "kubernetes"

	alertTypeReplicaHealth = "replica_health"
	alertTypeCrashLoop     = "crash_loop"
	alertTypeNodeCondition = "node_condition"

	entityWorkload = "workload"
	entityPod      = "pod"
	entityNode     = "node"

	defaultReplicaGracePeriod = 5 * time.Minute
	defaultCrashLoopRestarts  = 3
	defaultCrashLoopWindow    = 10 * time.Minute
)

// K8sAlertCallback is the function signature for emitting alert events.
type K8sAlertCallback func(evt alert.Event)

// K8sEventCallback broadcasts the SSE event of a workload, pod or node whose alert state changed.
type K8sEventCallback func(eventType string, data any)

// K8sAlertChecker detects alert conditions in Kubernetes workloads, pods and nodes.
type K8sAlertChecker struct {
	mu      sync.Mutex
	logger  *slog.Logger
	alertCb K8sAlertCallback
	eventCb K8sEventCallback
	now     func() time.Time

	replicas map[string]*replicaAlertState // workload ID
	pods     map[string]*podAlertState     // "namespace/name"
	nodes    map[string]*nodeAlertState    // node name
}

type replicaAlertState struct {
	firstSeen time.Time
	severity  string // last severity raised, empty during the grace period
}

type podAlertState struct {
	lastCount int32
	counted   bool
	restarts  []time.Time
	alerted   bool
}

type nodeAlertState struct {
	severity string
}

// NewK8sAlertChecker creates a new K8sAlertChecker.
func NewK8sAlertChecker(logger *slog.Logger) *K8sAlertChecker {
	return &K8sAlertChecker{
		logger:   logger,
		now:      time.Now,
		replicas: make(map[string]*replicaAlertState),
		pods:     make(map[string]*podAlertState),
		nodes:    make(map[string]*nodeAlertState),
	}
}

// SetAlertCallback sets the callback for emitting alerts.
func (c *K8sAlertChecker) SetAlertCallback(cb K8sAlertCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alertCb = cb
}

// SetEventCallback sets the callback for broadcasting SSE events.
func (c *K8sAlertChecker) SetEventCallback(cb K8sEventCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventCb = cb
}

// Resume takes over the active Kubernetes alerts left by a previous run, so the
// next check resolves those whose condition is gone.
func (c *K8sAlertChecker) Resume(active []*alert.Alert) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for _, a := range active {
		if a.Source != alertSource {
			continue
		}
		switch {
		case a.AlertType == alertTypeReplicaHealth && a.EntityType == entityWorkload:
			c.replicas[a.EntityID] = &replicaAlertState{firstSeen: now.Add(-defaultReplicaGracePeriod), severity: a.Severity}
		case a.AlertType == alertTypeCrashLoop && a.EntityType == entityPod:
			c.pods[a.EntityID] = &podAlertState{alerted: true}
		case a.AlertType == alertTypeNodeCondition && a.EntityType == entityNode:
			c.nodes[a.EntityID] = &nodeAlertState{severity: a.Severity}
		}
	}
}

// Check evaluates a snapshot of the cluster: under-replicated workloads,
// crash-looping pods and unhealthy nodes raise an alert once, and resolve when
// the condition clears, the object is ignored or it no longer exists.
func (c *K8sAlertChecker) Check(snap TopologySnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.checkWorkloads(snap.Workloads, now)
	c.checkPods(snap.Pods, snap.Workloads, now)
	// An empty list means the nodes could not be listed, never a cluster without nodes.
	if len(snap.Nodes) > 0 {
		c.checkNodes(snap.Nodes, now)
	}
}

func (c *K8sAlertChecker) checkWorkloads(workloads []K8sWorkload, now time.Time) {
	present := make(map[string]bool, len(workloads))
	for _, wl := range workloads {
		present[wl.ID] = true
		st := c.replicas[wl.ID]

		if wl.Ignored || wl.Kind == "Job" || wl.DesiredReplicas == 0 || wl.ReadyReplicas >= wl.DesiredReplicas {
			if st == nil {
				continue
			}
			delete(c.replicas, wl.ID)
			if st.severity == "" {
				continue
			}
			msg := fmt.Sprintf("Workload %s recovered: %d/%d replicas ready", wl.Name, wl.ReadyReplicas, wl.DesiredReplicas)
			if wl.Ignored {
				msg = fmt.Sprintf("Workload %s is ignored", wl.Name)
			}
			c.emitWorkload(wl.ID, wl.Namespace, wl.Name, alert.Event{
				Severity:  alert.SeverityInfo,
				IsRecover: true,
				Message:   msg,
				Details:   workloadDetails(wl),
			}, now)
			continue
		}

		if st == nil {
			c.replicas[wl.ID] = &replicaAlertState{firstSeen: now}
			continue
		}
		if now.Sub(st.firstSeen) < defaultReplicaGracePeriod {
			continue
		}
		severity := alert.SeverityWarning
		if wl.ReadyReplicas == 0 {
			severity = alert.SeverityCritical
		}
		if st.severity == severity || st.severity == alert.SeverityCritical {
			continue
		}
		st.severity = severity
		details := workloadDetails(wl)
		details["duration"] = now.Sub(st.firstSeen).Truncate(time.Second).String()
		c.emitWorkload(wl.ID, wl.Namespace, wl.Name, alert.Event{
			Severity: severity,
			Message:  fmt.Sprintf("Workload %s under-replicated for %s: %d/%d ready", wl.Name, now.Sub(st.firstSeen).Truncate(time.Second), wl.ReadyReplicas, wl.DesiredReplicas),
			Details:  details,
		}, now)
	}

	for id, st := range c.replicas {
		if present[id] {
			continue
		}
		delete(c.replicas, id)
		if st.severity == "" {
			continue
		}
		namespace, _, name, _ := parseExternalID(id)
		c.emitWorkload(id, namespace, name, alert.Event{
			Severity:  alert.SeverityInfo,
			IsRecover: true,
			Message:   fmt.Sprintf("Workload %s no longer exists", id),
		}, now)
	}
}

func workloadDetails(wl K8sWorkload) map[string]any {
	return map[string]any{
		"namespace":        wl.Namespace,
		"kind":             wl.Kind,
		"ready_replicas":   wl.ReadyReplicas,
		"desired_replicas": wl.DesiredReplicas,
	}
}

// checkPods raises a crash_loop alert for a pod in CrashLoopBackOff, or that
// restarted defaultCrashLoopRestarts times within defaultCrashLoopWindow.
func (c *K8sAlertChecker) checkPods(pods []K8sPod, workloads []K8sWorkload, now time.Time) {
	ignoredWorkloads := make(map[string]bool)
	for _, wl := range workloads {
		if wl.Ignored {
			ignoredWorkloads[wl.ID] = true
		}
	}

	cutoff := now.Add(-defaultCrashLoopWindow)
	present := make(map[string]bool, len(pods))
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		present[key] = true
		st := c.pods[key]
		if st == nil {
			st = &podAlertState{}
			c.pods[key] = st
		}

		if st.counted && pod.RestartCount > st.lastCount {
			for range min(pod.RestartCount-st.lastCount, defaultCrashLoopRestarts) {
				st.restarts = append(st.restarts, now)
			}
		}
		st.lastCount, st.counted = pod.RestartCount, true
		recent := st.restarts[:0]
		for _, t := range st.restarts {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		st.restarts = recent

		ignored := pod.Ignored || ignoredWorkloads[pod.WorkloadRef]
		crashLooping := pod.StatusReason == "CrashLoopBackOff" || len(st.restarts) >= defaultCrashLoopRestarts
		details := map[string]any{
			"namespace":     pod.Namespace,
			"restart_count": pod.RestartCount,
			"status_reason": pod.StatusReason,
			"node_name":     pod.NodeName,
		}

		switch {
		case ignored || !crashLooping:
			if !st.alerted {
				continue
			}
			st.alerted = false
			msg := fmt.Sprintf("Pod %s is no longer crash-looping", key)
			if ignored {
				msg = fmt.Sprintf("Pod %s is ignored", key)
			}
			c.emitPod(pod.Namespace, pod.Name, alert.Event{
				Severity:  alert.SeverityInfo,
				IsRecover: true,
				Message:   msg,
				Details:   details,
			}, now)
		case !st.alerted:
			st.alerted = true
			msg := fmt.Sprintf("Pod %s is in CrashLoopBackOff (%d restarts)", key, pod.RestartCount)
			if pod.StatusReason != "CrashLoopBackOff" {
				msg = fmt.Sprintf("Pod %s in crash loop: %d restarts in %s", key, len(st.restarts), defaultCrashLoopWindow)
			}
			c.emitPod(pod.Namespace, pod.Name, alert.Event{
				Severity: alert.SeverityCritical,
				Message:  msg,
				Details:  details,
			}, now)
		}
	}

	for key, st := range c.pods {
		if present[key] {
			continue
		}
		delete(c.pods, key)
		if !st.alerted {
			continue
		}
		namespace, name, _ := strings.Cut(key, "/")
		c.emitPod(namespace, name, alert.Event{
			Severity:  alert.SeverityInfo,
			IsRecover: true,
			Message:   fmt.Sprintf("Pod %s no longer exists", key),
		}, now)
	}
}

// checkNodes raises one node_condition alert per node: critical while the node
// is not Ready, warning under memory, disk or PID pressure.
func (c *K8sAlertChecker) checkNodes(nodes []K8sNode, now time.Time) {
	present := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		present[node.Name] = true
		conditions, severity := nodeProblems(node)
		st := c.nodes[node.Name]

		if len(conditions) == 0 {
			if st == nil {
				continue
			}
			delete(c.nodes, node.Name)
			c.emitNode(node.Name, alert.Event{
				Severity:  alert.SeverityInfo,
				IsRecover: true,
				Message:   fmt.Sprintf("Node %s recovered", node.Name),
				Details:   map[string]any{"roles": node.Roles},
			}, now)
			continue
		}

		if st == nil {
			st = &nodeAlertState{}
			c.nodes[node.Name] = st
		}
		if st.severity == severity || st.severity == alert.SeverityCritical {
			continue
		}
		st.severity = severity
		c.emitNode(node.Name, alert.Event{
			Severity: severity,
			Message:  fmt.Sprintf("Node %s: %s", node.Name, strings.Join(conditions, ", ")),
			Details:  map[string]any{"conditions": conditions, "roles": node.Roles},
		}, now)
	}

	for name := range c.nodes {
		if present[name] {
			continue
		}
		delete(c.nodes, name)
		c.emitNode(name, alert.Event{
			Severity:  alert.SeverityInfo,
			IsRecover: true,
			Message:   fmt.Sprintf("Node %s is no longer in the cluster", name),
		}, now)
	}
}

func nodeProblems(node K8sNode) (conditions []string, severity string) {
	if node.Status != "ready" {
		conditions = append(conditions, "NotReady")
		severity = alert.SeverityCritical
	}
	for _, cond := range node.Conditions {
		switch cond.Type {
		case "MemoryPressure", "DiskPressure", "PIDPressure":
			if cond.Status != "True" {
				continue
			}
			conditions = append(conditions, cond.Type)
			if severity == "" {
				severity = alert.SeverityWarning
			}
		}
	}
	return conditions, severity
}

func (c *K8sAlertChecker) emitWorkload(id, namespace, name string, evt alert.Event, now time.Time) {
	evt.AlertType, evt.EntityType, evt.EntityID, evt.EntityName = alertTypeReplicaHealth, entityWorkload, id, id
	c.emit(evt, now, event.KubernetesWorkloadChanged, map[string]any{"id": id, "namespace": namespace, "name": name})
}

func (c *K8sAlertChecker) emitPod(namespace, name string, evt alert.Event, now time.Time) {
	key := namespace + "/" + name
	evt.AlertType, evt.EntityType, evt.EntityID, evt.EntityName = alertTypeCrashLoop, entityPod, key, key
	c.emit(evt, now, event.KubernetesPodChanged, map[string]any{"namespace": namespace, "name": name})
}

func (c *K8sAlertChecker) emitNode(name string, evt alert.Event, now time.Time) {
	evt.AlertType, evt.EntityType, evt.EntityID, evt.EntityName = alertTypeNodeCondition, entityNode, name, name
	c.emit(evt, now, event.KubernetesNodeChanged, map[string]any{"name": name})
}

func (c *K8sAlertChecker) emit(evt alert.Event, now time.Time, eventType string, data map[string]any) {
	evt.Source = alertSource
	evt.Timestamp = now
	if c.alertCb != nil {
		c.alertCb(evt)
	} else {
		c.logger.Info("k8s alert (no callback)", "type", evt.AlertType, "severity", evt.Severity, "message", evt.Message)
	}
	if c.eventCb != nil {
		c.eventCb(eventType, data)
	}
}
