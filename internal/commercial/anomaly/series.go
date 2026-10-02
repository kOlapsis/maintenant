// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"slices"

	model "github.com/kolapsis/maintenant/internal/anomaly"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

// ScopeID derives the logical scope a container belongs to, stable across recreation, reschedule and replica changes.
func ScopeID(id model.ScopeIdentity) string {
	switch {
	case id.SwarmServiceName != "" || id.ControllerKind == "swarm-service":
		return "swarm/" + id.SwarmServiceName
	case id.RuntimeType == "kubernetes":
		return "k8s/" + id.Namespace + "/" + id.ControllerKind + "/" + id.OrchestrationUnit
	case id.OrchestrationGroup != "" && id.OrchestrationUnit != "":
		return "compose/" + id.OrchestrationGroup + "/" + id.OrchestrationUnit
	default:
		return "container/" + id.Name
	}
}

// ContainerScopeID returns the scope a discovered container belongs to.
func ContainerScopeID(c *container.Container) string {
	return ScopeID(model.ScopeIdentity{
		Name:               c.Name,
		RuntimeType:        c.RuntimeType,
		OrchestrationGroup: c.OrchestrationGroup,
		OrchestrationUnit:  c.OrchestrationUnit,
		ControllerKind:     c.ControllerKind,
		Namespace:          c.Namespace,
		SwarmServiceName:   c.SwarmServiceName,
	})
}

// NodeID returns the host a series belongs to, the local sentinel for the server's own runtime.
func NodeID(agentID string) string {
	return uid.Agent(agentID)
}

var containerMetrics = []string{model.MetricCPU, model.MetricMemory, model.MetricNetworkIO, model.MetricDiskIO}

var hostMetrics = []string{
	model.MetricCPU, model.MetricMemory, model.MetricLoad, model.MetricSwap,
	model.MetricDiskSpace, model.MetricDiskIO, model.MetricNetworkIO,
}

// MetricsForScope returns every metric a scope type may carry.
func MetricsForScope(scopeType string) []string {
	if scopeType == model.ScopeTypeHost {
		return slices.Clone(hostMetrics)
	}
	return slices.Clone(containerMetrics)
}

// IsMetricValid reports whether metric is meaningful for scopeType.
func IsMetricValid(scopeType, metric string) bool {
	if scopeType == model.ScopeTypeHost {
		return slices.Contains(hostMetrics, metric)
	}
	return slices.Contains(containerMetrics, metric)
}

func keyStr(k model.SeriesKey) string {
	return k.ScopeType + "|" + k.ScopeID + "|" + k.Metric + "|" + k.Dimension
}
