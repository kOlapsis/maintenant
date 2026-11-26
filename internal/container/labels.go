// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const (
	labelStackNamespace   = "com.docker.stack.namespace"
	labelSwarmServiceID   = "com.docker.swarm.service.id"
	labelSwarmServiceName = "com.docker.swarm.service.name"
	labelSwarmNodeID      = "com.docker.swarm.node.id"
	labelSwarmTaskID      = "com.docker.swarm.task.id"
	labelSwarmTaskName    = "com.docker.swarm.task.name"
)

// ControllerSwarmService is the controller kind of a Swarm task container.
const ControllerSwarmService = "swarm-service"

// ApplySwarmTaskLabels fills the Swarm service, node and slot of a task
// container from the labels Docker sets on it.
func (c *Container) ApplySwarmTaskLabels(labels map[string]string) {
	c.SwarmServiceID = labels[labelSwarmServiceID]
	c.SwarmServiceName = labels[labelSwarmServiceName]
	c.SwarmNodeID = labels[labelSwarmNodeID]
	c.SwarmTaskSlot = swarmTaskSlot(labels)
	if c.SwarmServiceID != "" {
		c.ControllerKind = ControllerSwarmService
	}
}

// ReplacedSwarmTasks returns the IDs of the exited or dead Swarm task containers whose service still runs a task container.
func ReplacedSwarmTasks(cs []*Container) map[string]bool {
	running := make(map[string]bool)
	for _, c := range cs {
		if c.ControllerKind == ControllerSwarmService && c.State == StateRunning {
			running[c.SwarmServiceID] = true
		}
	}
	replaced := make(map[string]bool)
	for _, c := range cs {
		if c.ControllerKind == ControllerSwarmService && (c.State == StateExited || c.State == StateDead) && running[c.SwarmServiceID] {
			replaced[c.ID] = true
		}
	}
	return replaced
}

// swarmTaskSlot reads the slot from a task name, <service>.<slot>.<task id>;
// a global service puts its node ID there and has no slot.
func swarmTaskSlot(labels map[string]string) int {
	name := strings.TrimPrefix(labels[labelSwarmTaskName], labels[labelSwarmServiceName]+".")
	name = strings.TrimSuffix(name, "."+labels[labelSwarmTaskID])
	slot, err := strconv.Atoi(name)
	if err != nil || slot < 1 {
		return 0
	}
	return slot
}

// IgnoredByLabels reports whether labels mark a container with maintenant.ignore.
func IgnoredByLabels(labels map[string]string) bool {
	v := labels[labelPBIgnore]
	return v == "true" || v == "1"
}

// OrchestrationGroupFromLabels returns the Compose project of a container, or
// its Swarm stack when Compose did not start it.
func OrchestrationGroupFromLabels(labels map[string]string) string {
	if project := labels[labelComposeProject]; project != "" {
		return project
	}
	return labels[labelStackNamespace]
}

// adoptLabelFields copies from d the fields read from labels or annotations and
// reports whether any changed.
func (c *Container) adoptLabelFields(d *Container) bool {
	changed := c.IsIgnored != d.IsIgnored || c.CustomGroup != d.CustomGroup ||
		c.AlertSeverity != d.AlertSeverity || c.RestartThreshold != d.RestartThreshold ||
		c.OrchestrationGroup != d.OrchestrationGroup || c.ControllerKind != d.ControllerKind ||
		c.SwarmServiceID != d.SwarmServiceID || c.SwarmServiceName != d.SwarmServiceName ||
		c.SwarmNodeID != d.SwarmNodeID || c.SwarmTaskSlot != d.SwarmTaskSlot
	c.IsIgnored = d.IsIgnored
	c.CustomGroup = d.CustomGroup
	c.AlertSeverity = d.AlertSeverity
	c.RestartThreshold = d.RestartThreshold
	c.OrchestrationGroup = d.OrchestrationGroup
	c.ControllerKind = d.ControllerKind
	c.SwarmServiceID = d.SwarmServiceID
	c.SwarmServiceName = d.SwarmServiceName
	c.SwarmNodeID = d.SwarmNodeID
	c.SwarmTaskSlot = d.SwarmTaskSlot
	return changed
}

// agentLabelFields returns the label-derived fields of a remote agent's container.
func agentLabelFields(labels map[string]string) *Container {
	c := &Container{
		OrchestrationGroup: OrchestrationGroupFromLabels(labels),
		AlertSeverity:      SeverityWarning,
		RestartThreshold:   3,
	}
	applyAgentLabels(c, labels)
	c.ApplySwarmTaskLabels(labels)
	return c
}

var knownLabelKeys = map[string]struct{}{
	labelPBIgnore:                 {},
	labelPBGroup:                  {},
	labelPBSeverity:               {},
	labelPBThreshold:              {},
	"maintenant.tls.certificates": {},
	"maintenant.proxy-labels":     {},
}

var knownLabelPrefixes = []string{"maintenant.endpoint.", UpdateLabelPrefix}

var knownLabelValues = map[string]func(string) bool{
	labelPBIgnore: func(v string) bool { return v == "true" || v == "1" || v == "false" || v == "0" },
	labelPBSeverity: func(v string) bool {
		switch AlertSeverity(v) {
		case SeverityCritical, SeverityWarning, SeverityInfo:
			return true
		}
		return false
	},
	labelPBThreshold: func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n > 0
	},
}

var unknownLabelsWarned sync.Map

func isKnownLabel(key string) bool {
	if _, ok := knownLabelKeys[key]; ok {
		return true
	}
	for _, p := range knownLabelPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// WarnUnknownLabels logs a warning for each unrecognised maintenant.* label on a container, and for each known one whose value cannot be used, once per container per process.
func WarnUnknownLabels(containerID string, labels map[string]string, logger *slog.Logger) {
	if _, done := unknownLabelsWarned.Load(containerID); done {
		return
	}
	var unknown, invalid []string
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		if !strings.HasPrefix(key, "maintenant.") {
			continue
		}
		if !isKnownLabel(key) {
			unknown = append(unknown, key)
		} else if valid, checked := knownLabelValues[key]; checked && !valid(labels[key]) {
			invalid = append(invalid, key)
		}
	}
	if len(unknown)+len(invalid) == 0 {
		return
	}
	if _, done := unknownLabelsWarned.LoadOrStore(containerID, struct{}{}); done {
		return
	}
	who := shortID(containerID)
	if strings.Contains(containerID, "/") { // a Kubernetes namespace/Kind/name, not a Docker ID
		who = containerID
	}
	for _, key := range unknown {
		logger.Warn("unknown maintenant.* label ignored", "container", who, "label", key)
	}
	for _, key := range invalid {
		logger.Warn("maintenant.* label with an invalid value ignored", "container", who, "label", key, "value", labels[key])
	}
}
