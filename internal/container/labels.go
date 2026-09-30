// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"strconv"
	"strings"
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
