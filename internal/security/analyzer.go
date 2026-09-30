// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

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
func AnalyzeDocker(containerID string, containerName string, cfg DockerSecurityConfig, now time.Time) []Insight {
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

func analyzePortBindings(containerID string, containerName string, bindings []PortBinding, now time.Time) []Insight {
	var insights []Insight
	for _, b := range bindings {
		if !isExposedOnAllInterfaces(b.HostIP) {
			continue
		}

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
		} else {
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
		}
	}
	return insights
}

// Kubernetes Service types that open a port outside the cluster.
const (
	ServiceTypeLoadBalancer = "LoadBalancer"
	ServiceTypeNodePort     = "NodePort"
)

// ServicePort is one port a LoadBalancer or NodePort Service exposes for a workload.
type ServicePort struct {
	Service    string // namespace/name
	Type       string // ServiceTypeLoadBalancer or ServiceTypeNodePort
	Port       int
	TargetPort int // 0 when the Service targets a named port
	NodePort   int
	Protocol   string
}

// AnalyzeKubernetes returns the insights of the ports Services expose for a workload.
func AnalyzeKubernetes(containerID string, containerName string, ports []ServicePort, now time.Time) []Insight {
	var insights []Insight
	for _, p := range ports {
		port := p.TargetPort
		if port == 0 {
			port = p.Port
		}
		details := map[string]any{"port": port, "protocol": p.Protocol, "service": p.Service, "service_type": p.Type}
		if p.NodePort != 0 {
			details["node_port"] = p.NodePort
		}
		insight := Insight{
			Severity:      SeverityCritical,
			ContainerID:   containerID,
			ContainerName: containerName,
			Details:       details,
			DetectedAt:    now,
		}

		switch dbType, isDB := knownDatabasePorts[port]; {
		case isDB:
			insight.Type = DatabasePortExposed
			insight.Title = "Database port publicly exposed"
			insight.Description = fmt.Sprintf("%s port %d is reachable from outside the cluster through %s service %s.",
				dbType, port, p.Type, p.Service)
			details["database_type"] = dbType
		case p.Type == ServiceTypeLoadBalancer:
			insight.Type = ServiceLoadBalancer
			insight.Title = "Port exposed by a LoadBalancer service"
			insight.Description = fmt.Sprintf("Port %d/%s is reachable from outside the cluster through LoadBalancer service %s.",
				port, p.Protocol, p.Service)
		case p.Type == ServiceTypeNodePort:
			insight.Type = ServiceNodePort
			insight.Title = "Port exposed on every node"
			insight.Description = fmt.Sprintf("Port %d/%s is published on port %d of every node by NodePort service %s.",
				port, p.Protocol, p.NodePort, p.Service)
		default:
			continue
		}
		insights = append(insights, insight)
	}
	return insights
}

func isExposedOnAllInterfaces(hostIP string) bool {
	return hostIP == "" || hostIP == "0.0.0.0" || hostIP == "::"
}

func isHostNetwork(mode string) bool {
	return mode == "host"
}
