// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

const labelStackNamespace = "com.docker.stack.namespace"

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
		c.OrchestrationGroup != d.OrchestrationGroup
	c.IsIgnored = d.IsIgnored
	c.CustomGroup = d.CustomGroup
	c.AlertSeverity = d.AlertSeverity
	c.RestartThreshold = d.RestartThreshold
	c.OrchestrationGroup = d.OrchestrationGroup
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
	return c
}
