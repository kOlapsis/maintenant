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
	labelMaintChannels  = "maintenant.alert.channels"
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
	if v, ok := serviceLabels[labelMaintChannels]; ok && v != "" {
		c.AlertChannels = v
	}

	// Stack grouping via com.docker.stack.namespace
	if stack := serviceLabels[labelStackNamespace]; stack != "" {
		c.OrchestrationGroup = stack
	}
}
