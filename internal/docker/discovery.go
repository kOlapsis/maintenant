// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

const (
	labelComposeService    = "com.docker.compose.service"
	labelComposeWorkingDir = "com.docker.compose.project.working_dir"
	labelComposeOneOff     = "com.docker.compose.oneoff"
	labelPBIgnore          = "maintenant.ignore"
	labelPBGroup           = "maintenant.group"
	labelPBSeverity        = "maintenant.alert.severity"
	labelPBThreshold       = "maintenant.alert.restart_threshold"
)

// SecurityConfig holds security-relevant fields extracted from Docker's ContainerInspect.
type SecurityConfig struct {
	Privileged   bool
	NetworkMode  string
	PortBindings []PortBindingInfo
}

// PortBindingInfo represents a single host port binding.
type PortBindingInfo struct {
	HostIP        string
	HostPort      string
	ContainerPort int
	Protocol      string
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
	Exit           *ExitInfo // nil unless the container has exited and inspect succeeded
}

// ExitInfo is how an exited container last stopped.
type ExitInfo struct {
	Code      int
	OOMKilled bool
}

// IsOneOff reports whether a container was created by `docker compose run`.
//
// Compose copies the service definition onto these containers, our labels
// included, but gives them a generated name. Since a label-discovered monitor
// is keyed on the container name, a one-off would mint a second monitor for a
// service that already has one, and that monitor outlives the container it came
// from. They are not part of the fleet, so discovery skips them entirely.
func IsOneOff(labels map[string]string) bool {
	return strings.EqualFold(labels[labelComposeOneOff], "true")
}

// DiscoverAll performs a full container list + inspect pass, returning all discovered containers.
func (c *Client) DiscoverAll(ctx context.Context) ([]*cmodel.Container, error) {
	res, err := c.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	now := time.Now()
	containers := make([]*cmodel.Container, 0, len(res.Items))

	for _, dc := range res.Items {
		if IsOneOff(dc.Labels) {
			continue
		}
		labels := c.containerLabels(ctx, dc.Labels)
		result, err := c.inspectAndMap(ctx, dc, labels, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			containers = append(containers, mapFromList(dc, labels, now))
			continue
		}
		containers = append(containers, result.Container)
	}

	return containers, nil
}

// DiscoverAllWithLabels is like DiscoverAll but also returns raw Docker labels
// and security configuration for each container.
func (c *Client) DiscoverAllWithLabels(ctx context.Context) ([]*DiscoveryResult, error) {
	res, err := c.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	now := time.Now()
	results := make([]*DiscoveryResult, 0, len(res.Items))

	for _, dc := range res.Items {
		if IsOneOff(dc.Labels) {
			continue
		}
		labels := c.containerLabels(ctx, dc.Labels)
		result, err := c.inspectAndMap(ctx, dc, labels, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			results = append(results, &DiscoveryResult{
				Container: mapFromList(dc, labels, now),
				Labels:    labels,
			})
			continue
		}
		results = append(results, &DiscoveryResult{
			Container:      result.Container,
			Labels:         labels,
			SecurityConfig: result.SecurityConfig,
			Exit:           result.Exit,
		})
	}

	return results, nil
}

// ContainerRepoDigests maps each container ID to the repo digests of the image it runs, empty for an image never pulled nor pushed.
func (c *Client) ContainerRepoDigests(ctx context.Context) (map[string][]string, error) {
	containers, err := c.cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}
	images, err := c.cli.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, fmt.Errorf("image list: %w", err)
	}

	digestsByImage := make(map[string][]string, len(images.Items))
	for _, img := range images.Items {
		digestsByImage[img.ID] = img.RepoDigests
	}

	out := make(map[string][]string, len(containers.Items))
	for _, dc := range containers.Items {
		if digests, ok := digestsByImage[dc.ImageID]; ok {
			out[dc.ID] = digests
		}
	}
	return out, nil
}

// inspectResult holds the mapped container along with its extracted security config.
type inspectResult struct {
	Container      *cmodel.Container
	SecurityConfig *SecurityConfig
	Exit           *ExitInfo
}

// inspectAndMap calls ContainerInspect and maps the result to our domain model.
func (c *Client) inspectAndMap(ctx context.Context, dc container.Summary, labels map[string]string, now time.Time) (*inspectResult, error) {
	res, err := c.cli.ContainerInspect(ctx, dc.ID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", dc.ID[:12], err)
	}
	info := res.Container

	cm := mapFromList(dc, labels, now)
	// The list shows the image ID instead once the tag the container was created from points elsewhere.
	if info.Config != nil && info.Config.Image != "" {
		cm.Image = info.Config.Image
	}

	// Health check info from inspect
	if info.Config != nil && info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 {
		cm.HasHealthCheck = true
	}
	if info.State != nil && info.State.Health != nil {
		hs := mapHealthStatus(string(info.State.Health.Status))
		cm.HealthStatus = &hs
	}

	var exit *ExitInfo
	if info.State != nil && info.State.Status == "exited" {
		exit = &ExitInfo{Code: info.State.ExitCode, OOMKilled: info.State.OOMKilled}
		if cmodel.IsCleanExit(exit.Code, exit.OOMKilled) {
			cm.State = cmodel.StateCompleted
		}
	}

	// Extract security-relevant config from HostConfig
	secCfg := extractSecurityConfig(info.HostConfig)

	return &inspectResult{Container: cm, SecurityConfig: secCfg, Exit: exit}, nil
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
			var hostIP string
			if b.HostIP.IsValid() {
				hostIP = b.HostIP.String()
			}
			cfg.PortBindings = append(cfg.PortBindings, PortBindingInfo{
				HostIP:        hostIP,
				HostPort:      b.HostPort,
				ContainerPort: int(port.Num()),
				Protocol:      string(port.Proto()),
			})
		}
	}

	return cfg
}

// mapFromList creates a Container from the docker ContainerList response and
// the container's effective labels.
func mapFromList(dc container.Summary, labels map[string]string, now time.Time) *cmodel.Container {
	name := ""
	if len(dc.Names) > 0 {
		name = dc.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
	}

	state := mapContainerState(string(dc.State))
	readyCount := 0
	if state == cmodel.StateRunning {
		readyCount = 1
	}

	cm := &cmodel.Container{
		ExternalID:         dc.ID,
		Name:               name,
		Image:              dc.Image,
		State:              state,
		OrchestrationGroup: cmodel.OrchestrationGroupFromLabels(labels),
		OrchestrationUnit:  labels[labelComposeService],
		ComposeWorkingDir:  labels[labelComposeWorkingDir],
		RuntimeType:        "docker",
		PodCount:           1,
		ReadyCount:         readyCount,
		AlertSeverity:      cmodel.SeverityWarning,
		RestartThreshold:   3,
		FirstSeenAt:        now,
		LastStateChangeAt:  now,
	}

	applyLabels(cm, labels)
	cm.ApplySwarmTaskLabels(labels)
	cm.ApplyImageLabels(labels)

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
