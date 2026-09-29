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
	"github.com/kolapsis/maintenant/internal/security"
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

		bindings := make([]security.PortBinding, 0, len(r.SecurityConfig.PortBindings))
		for _, pb := range r.SecurityConfig.PortBindings {
			bindings = append(bindings, security.PortBinding{
				HostIP:   pb.HostIP,
				HostPort: pb.HostPort,
				Port:     pb.ContainerPort,
				Protocol: pb.Protocol,
			})
		}
		insights := security.AnalyzeDocker(c.ID, c.Name, security.DockerSecurityConfig{
			Privileged:  r.SecurityConfig.Privileged,
			NetworkMode: r.SecurityConfig.NetworkMode,
			Bindings:    bindings,
		}, now)
		secSvc.UpdateContainer(c.ID, c.Name, insights)
		return
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
