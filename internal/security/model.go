// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package security

import "time"

// InsightType enumerates the categories of dangerous configurations.
type InsightType string

const (
	PortExposedAllInterfaces InsightType = "port_exposed_all_interfaces"
	DatabasePortExposed      InsightType = "database_port_exposed"
	PrivilegedContainer      InsightType = "privileged_container"
	HostNetworkMode          InsightType = "host_network_mode"
	ServiceLoadBalancer      InsightType = "service_load_balancer"
	ServiceNodePort          InsightType = "service_node_port"
	MissingNetworkPolicy     InsightType = "missing_network_policy"
)

// Severity levels for security insights.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
)

// Insight represents a single detected dangerous configuration.
type Insight struct {
	Type          InsightType    `json:"type"`
	Severity      string         `json:"severity"`
	ContainerID   int64          `json:"container_id"`
	ContainerName string         `json:"container_name"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Details       map[string]any `json:"details"`
	DetectedAt    time.Time      `json:"detected_at"`
}

// ContainerInsights holds all active insights for a single container.
type ContainerInsights struct {
	ContainerID     int64      `json:"container_id"`
	ContainerName   string     `json:"container_name"`
	HighestSeverity *string    `json:"highest_severity"`
	Count           int        `json:"count"`
	Insights        []Insight  `json:"insights"`
}

// Summary provides aggregated counts across all containers.
type Summary struct {
	TotalContainersMonitored int            `json:"total_containers_monitored"`
	TotalContainersAffected  int            `json:"total_containers_affected"`
	TotalInsights            int            `json:"total_insights"`
	BySeverity               map[string]int `json:"by_severity"`
	ByType                   map[string]int `json:"by_type"`
}

// severityRank returns a numeric rank for severity comparison (higher = more severe).
func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 1
	default:
		return 0
	}
}

// HighestSeverity returns the most severe level from a list of insights.
func HighestSeverity(insights []Insight) string {
	best := ""
	for _, i := range insights {
		if severityRank(i.Severity) > severityRank(best) {
			best = i.Severity
		}
	}
	return best
}
