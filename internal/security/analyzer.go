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

import (
	"fmt"
	"time"
)

// PortBinding represents a single host port binding from Docker HostConfig.
type PortBinding struct {
	HostIP   string
	HostPort string
	Port     int
	Protocol string
}

// DockerSecurityConfig holds the security-relevant fields extracted from Docker's ContainerInspect.
type DockerSecurityConfig struct {
	Privileged  bool
	NetworkMode string
	Bindings    []PortBinding
}

// knownDatabasePorts maps well-known database ports to their database type name.
var knownDatabasePorts = map[int]string{
	3306:  "MySQL/MariaDB",
	5432:  "PostgreSQL",
	6379:  "Redis",
	27017: "MongoDB",
}

// AnalyzeDocker inspects a Docker container's security configuration and returns all detected insights.
func AnalyzeDocker(containerID int64, containerName string, cfg DockerSecurityConfig, now time.Time) []Insight {
	var insights []Insight

	// Host network mode subsumes port binding checks.
	if isHostNetwork(cfg.NetworkMode) {
		insights = append(insights, Insight{
			Type:          HostNetworkMode,
			Severity:      SeverityHigh,
			ContainerID:   containerID,
			ContainerName: containerName,
			Title:         "Host network mode",
			Description:   "Container uses host network mode, sharing the host's network namespace.",
			Details:       map[string]any{"network_mode": cfg.NetworkMode},
			DetectedAt:    now,
		})
	} else {
		insights = append(insights, analyzePortBindings(containerID, containerName, cfg.Bindings, now)...)
	}

	if cfg.Privileged {
		insights = append(insights, Insight{
			Type:          PrivilegedContainer,
			Severity:      SeverityCritical,
			ContainerID:   containerID,
			ContainerName: containerName,
			Title:         "Privileged container",
			Description:   "Container runs in privileged mode with full access to the host.",
			Details:       map[string]any{},
			DetectedAt:    now,
		})
	}

	return insights
}

func analyzePortBindings(containerID int64, containerName string, bindings []PortBinding, now time.Time) []Insight {
	var insights []Insight
	for _, b := range bindings {
		if !isExposedOnAllInterfaces(b.HostIP) {
			continue
		}

		insights = append(insights, Insight{
			Type:          PortExposedAllInterfaces,
			Severity:      SeverityCritical,
			ContainerID:   containerID,
			ContainerName: containerName,
			Title:         "Port exposed on all interfaces",
			Description:   fmt.Sprintf("Port %d/%s is bound to 0.0.0.0, making it accessible from any network interface.", b.Port, b.Protocol),
			Details:       map[string]any{"port": b.Port, "protocol": b.Protocol},
			DetectedAt:    now,
		})

		if dbType, ok := knownDatabasePorts[b.Port]; ok {
			insights = append(insights, Insight{
				Type:          DatabasePortExposed,
				Severity:      SeverityCritical,
				ContainerID:   containerID,
				ContainerName: containerName,
				Title:         "Database port publicly exposed",
				Description:   fmt.Sprintf("%s port %d is exposed on all interfaces (0.0.0.0).", dbType, b.Port),
				Details:       map[string]any{"port": b.Port, "protocol": b.Protocol, "database_type": dbType},
				DetectedAt:    now,
			})
		}
	}
	return insights
}

func isExposedOnAllInterfaces(hostIP string) bool {
	return hostIP == "" || hostIP == "0.0.0.0" || hostIP == "::"
}

func isHostNetwork(mode string) bool {
	return mode == "host"
}
