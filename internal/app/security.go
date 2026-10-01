// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/uid"
)

// ScanContainerSecurity inspects a single container and updates its security insights.
func ScanContainerSecurity(ctx context.Context, dr *docker.Runtime, containerSvc *container.Service, secSvc *security.Service, externalID string, logger *slog.Logger) {
	results, err := dr.DiscoverAllWithLabels(ctx)
	if err != nil {
		logger.Warn("security: failed to scan container", "external_id", externalID, "error", err)
		return
	}

	now := time.Now()
	for _, r := range results {
		if r.Container.ExternalID != externalID || r.SecurityConfig == nil {
			continue
		}
		c, err := containerSvc.GetContainer(ctx, r.Container.ID)
		if err != nil || c == nil {
			stored, _ := containerSvc.ListContainers(ctx, container.ListContainersOpts{IncludeIgnored: true})
			for _, sc := range stored {
				if sc.ExternalID == externalID {
					c = sc
					break
				}
			}
		}
		if c == nil {
			return
		}
		secSvc.UpdateContainer(c.ID, c.Name, dockerInsights(c, r.SecurityConfig, now))
		return
	}
}

// dockerInsights analyses a container's Docker configuration; an ignored
// container has none.
func dockerInsights(c *container.Container, cfg *docker.SecurityConfig, now time.Time) []security.Insight {
	if c.IsIgnored {
		return nil
	}
	bindings := make([]security.PortBinding, 0, len(cfg.PortBindings))
	for _, pb := range cfg.PortBindings {
		bindings = append(bindings, security.PortBinding{
			HostIP:   pb.HostIP,
			HostPort: pb.HostPort,
			Port:     pb.ContainerPort,
			Protocol: pb.Protocol,
		})
	}
	return security.AnalyzeDocker(c.ID, c.Name, security.DockerSecurityConfig{
		Privileged:  cfg.Privileged,
		NetworkMode: cfg.NetworkMode,
		Bindings:    bindings,
	}, now)
}

// serviceExposureSource lists the ports Kubernetes Services open outside the cluster.
type serviceExposureSource interface {
	ListServiceExposures(ctx context.Context) ([]kubernetes.ServiceExposure, error)
}

type containerLister interface {
	ListContainers(ctx context.Context, opts container.ListContainersOpts) ([]*container.Container, error)
}

// ScanKubernetesSecurity refreshes the insights of the local cluster's
// workloads from the LoadBalancer and NodePort Services that expose them.
func ScanKubernetesSecurity(ctx context.Context, src serviceExposureSource, containers containerLister, secSvc *security.Service, logger *slog.Logger) {
	exposures, err := src.ListServiceExposures(ctx)
	if err != nil {
		logger.Warn("security: kubernetes services not analysed", "error", err)
		return
	}
	byWorkload := make(map[string][]security.ServicePort, len(exposures))
	for _, e := range exposures {
		byWorkload[e.WorkloadID] = append(byWorkload[e.WorkloadID], security.ServicePort{
			Service:    e.Service,
			Type:       e.ServiceType,
			Port:       e.Port,
			TargetPort: e.TargetPort,
			NodePort:   e.NodePort,
			Protocol:   e.Protocol,
		})
	}

	local := uid.LocalAgent
	workloads, err := containers.ListContainers(ctx, container.ListContainersOpts{IncludeIgnored: true, AgentFilter: &local})
	if err != nil {
		logger.Warn("security: kubernetes workloads not listed", "error", err)
		return
	}
	now := time.Now()
	for _, c := range workloads {
		if c.RuntimeType != "kubernetes" {
			continue
		}
		var insights []security.Insight
		if !c.IsIgnored {
			insights = security.AnalyzeKubernetes(c.ID, c.Name, byWorkload[c.ExternalID], now)
		}
		secSvc.UpdateContainer(c.ID, c.Name, insights)
	}
}

// MapSecuritySeverity maps security insight severity to alert severity.
func MapSecuritySeverity(s string) string {
	switch s {
	case security.SeverityCritical:
		return alert.SeverityCritical
	case security.SeverityHigh:
		return alert.SeverityWarning
	case security.SeverityMedium:
		return alert.SeverityInfo
	default:
		return alert.SeverityInfo
	}
}
