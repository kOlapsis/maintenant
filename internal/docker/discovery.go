// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	cmodel "github.com/kolapsis/maintenant/internal/container"
)

const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
	labelPBIgnore       = "maintenant.ignore"
	labelPBGroup        = "maintenant.group"
	labelPBSeverity     = "maintenant.alert.severity"
	labelPBThreshold    = "maintenant.alert.restart_threshold"
	labelPBChannels     = "maintenant.alert.channels"
)

// SecurityConfig holds security-relevant fields extracted from Docker's ContainerInspect.
type SecurityConfig struct {
	Privileged  bool
	NetworkMode string
	PortBindings []PortBindingInfo
}

// PortBindingInfo represents a single host port binding.
type PortBindingInfo struct {
	HostIP       string
	HostPort     string
	ContainerPort int
	Protocol     string
}

// DiscoveredContainer holds the result of discovering a single container.
type DiscoveredContainer struct {
	Container *cmodel.Container
	Err       error
}

// DiscoveryResult holds a discovered container along with its raw labels for endpoint extraction.
type DiscoveryResult struct {
	Container      *cmodel.Container
	Labels         map[string]string
	SecurityConfig *SecurityConfig
}

// DiscoverAll performs a full container list + inspect pass, returning all discovered containers.
func (c *Client) DiscoverAll(ctx context.Context) ([]*cmodel.Container, error) {
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	now := time.Now()
	containers := make([]*cmodel.Container, 0, len(list))

	for _, dc := range list {
		result, err := c.inspectAndMap(ctx, dc, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			containers = append(containers, mapFromList(dc, now))
			continue
		}
		containers = append(containers, result.Container)
	}

	return containers, nil
}

// DiscoverAllWithLabels is like DiscoverAll but also returns raw Docker labels
// and security configuration for each container.
func (c *Client) DiscoverAllWithLabels(ctx context.Context) ([]*DiscoveryResult, error) {
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	now := time.Now()
	results := make([]*DiscoveryResult, 0, len(list))

	for _, dc := range list {
		result, err := c.inspectAndMap(ctx, dc, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			results = append(results, &DiscoveryResult{
				Container: mapFromList(dc, now),
				Labels:    dc.Labels,
			})
			continue
		}
		results = append(results, &DiscoveryResult{
			Container:      result.Container,
			Labels:         dc.Labels,
			SecurityConfig: result.SecurityConfig,
		})
	}

	return results, nil
}

// inspectResult holds the mapped container along with its extracted security config.
type inspectResult struct {
	Container      *cmodel.Container
	SecurityConfig *SecurityConfig
}

// inspectAndMap calls ContainerInspect and maps the result to our domain model.
func (c *Client) inspectAndMap(ctx context.Context, dc types.Container, now time.Time) (*inspectResult, error) {
	info, err := c.cli.ContainerInspect(ctx, dc.ID)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", dc.ID[:12], err)
	}

	cm := mapFromList(dc, now)

	// Health check info from inspect
	if info.Config != nil && info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 {
		cm.HasHealthCheck = true
	}
	if info.State != nil && info.State.Health != nil {
		hs := mapHealthStatus(info.State.Health.Status)
		cm.HealthStatus = &hs
	}

	// Reclassify gracefully-stopped containers as "completed" (normal termination).
	// Exit 0 = normal exit, 137 = SIGKILL (docker stop), 143 = SIGTERM.
	if info.State != nil && info.State.Status == "exited" && isGracefulExitCode(info.State.ExitCode) {
		cm.State = cmodel.StateCompleted
	}

	// Extract security-relevant config from HostConfig
	secCfg := extractSecurityConfig(info.HostConfig)

	return &inspectResult{Container: cm, SecurityConfig: secCfg}, nil
}

// extractSecurityConfig extracts security-relevant fields from Docker's HostConfig.
func extractSecurityConfig(hc *container.HostConfig) *SecurityConfig {
	if hc == nil {
		return &SecurityConfig{}
	}

	cfg := &SecurityConfig{
		Privileged:  hc.Privileged,
		NetworkMode: string(hc.NetworkMode),
	}

	for port, bindings := range hc.PortBindings {
		for _, b := range bindings {
			cfg.PortBindings = append(cfg.PortBindings, PortBindingInfo{
				HostIP:        b.HostIP,
				HostPort:      b.HostPort,
				ContainerPort: port.Int(),
				Protocol:      port.Proto(),
			})
		}
	}

	return cfg
}

// mapFromList creates a Container from the docker ContainerList response.
func mapFromList(dc types.Container, now time.Time) *cmodel.Container {
	name := ""
	if len(dc.Names) > 0 {
		name = dc.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
	}

	state := mapContainerState(dc.State)
	readyCount := 0
	if state == cmodel.StateRunning {
		readyCount = 1
	}

	cm := &cmodel.Container{
		ExternalID:         dc.ID,
		Name:               name,
		Image:              dc.Image,
		State:              state,
		OrchestrationGroup: dc.Labels[labelComposeProject],
		OrchestrationUnit:  dc.Labels[labelComposeService],
		RuntimeType:        "docker",
		PodCount:           1,
		ReadyCount:         readyCount,
		AlertSeverity:      cmodel.SeverityWarning,
		RestartThreshold:   3,
		FirstSeenAt:        now,
		LastStateChangeAt:  now,
	}

	// maintenant labels
	applyLabels(cm, dc.Labels)

	return cm
}

func applyLabels(cm *cmodel.Container, labels map[string]string) {
	if v, ok := labels[labelPBIgnore]; ok && (v == "true" || v == "1") {
		cm.IsIgnored = true
	}
	if v, ok := labels[labelPBGroup]; ok && v != "" {
		cm.CustomGroup = v
	}
	if v, ok := labels[labelPBSeverity]; ok {
		switch cmodel.AlertSeverity(v) {
		case cmodel.SeverityCritical, cmodel.SeverityWarning, cmodel.SeverityInfo:
			cm.AlertSeverity = cmodel.AlertSeverity(v)
		}
	}
	if v, ok := labels[labelPBThreshold]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cm.RestartThreshold = n
		}
	}
	if v, ok := labels[labelPBChannels]; ok && v != "" {
		cm.AlertChannels = v
	}
}

func mapContainerState(state string) cmodel.ContainerState {
	switch cmodel.ContainerState(state) {
	case cmodel.StateRunning, cmodel.StateExited, cmodel.StateCompleted, cmodel.StateRestarting,
		cmodel.StatePaused, cmodel.StateCreated, cmodel.StateDead:
		return cmodel.ContainerState(state)
	default:
		return cmodel.StateCreated
	}
}

// isGracefulExitCode returns true for exit codes that indicate a voluntary stop:
// 0 = normal, 137 = SIGKILL (docker stop), 143 = SIGTERM.
func isGracefulExitCode(code int) bool {
	return code == 0 || code == 137 || code == 143
}

func mapHealthStatus(status string) cmodel.HealthStatus {
	switch cmodel.HealthStatus(status) {
	case cmodel.HealthHealthy, cmodel.HealthUnhealthy, cmodel.HealthStarting:
		return cmodel.HealthStatus(status)
	default:
		return cmodel.HealthStarting
	}
}

// Logger returns the client logger for use in event processing.
func (c *Client) Logger() *slog.Logger {
	return c.logger
}
