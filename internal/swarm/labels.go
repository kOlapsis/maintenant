// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"strconv"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

const (
	// Docker Swarm built-in labels.
	labelStackNamespace = "com.docker.stack.namespace"
	labelSwarmServiceID = "com.docker.swarm.service.id"

	// Maintenant labels (applied at service level in Swarm).
	labelMaintGroup     = "maintenant.group"
	labelMaintIgnore    = "maintenant.ignore"
	labelMaintSeverity  = "maintenant.alert.severity"
	labelMaintThreshold = "maintenant.alert.restart_threshold"
)

// IsSwarmManaged returns true if the container has a Swarm service ID label.
func IsSwarmManaged(labels map[string]string) bool {
	_, ok := labels[labelSwarmServiceID]
	return ok
}

// StackName extracts the stack namespace from labels.
func StackName(labels map[string]string) string {
	return labels[labelStackNamespace]
}

// ApplyServiceLabels maps Swarm service-level labels to Container model fields.
// This applies maintenant.* labels from the service definition to the container.
func ApplyServiceLabels(c *cmodel.Container, serviceLabels map[string]string) {
	if v, ok := serviceLabels[labelMaintGroup]; ok && v != "" {
		c.CustomGroup = v
	}
	if v, ok := serviceLabels[labelMaintIgnore]; ok && (v == "true" || v == "1") {
		c.IsIgnored = true
	}
	if v, ok := serviceLabels[labelMaintSeverity]; ok {
		switch cmodel.AlertSeverity(v) {
		case cmodel.SeverityCritical, cmodel.SeverityWarning, cmodel.SeverityInfo:
			c.AlertSeverity = cmodel.AlertSeverity(v)
		}
	}
	if v, ok := serviceLabels[labelMaintThreshold]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.RestartThreshold = n
		}
	}

	// Stack grouping via com.docker.stack.namespace
	if stack := serviceLabels[labelStackNamespace]; stack != "" {
		c.OrchestrationGroup = stack
	}
}
