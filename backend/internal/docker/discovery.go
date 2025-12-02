package docker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	cmodel "github.com/kolapsis/pulseboard/internal/container"
)

const (
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
	labelPBIgnore       = "pulseboard.ignore"
	labelPBGroup        = "pulseboard.group"
	labelPBSeverity     = "pulseboard.alert.severity"
	labelPBThreshold    = "pulseboard.alert.restart_threshold"
	labelPBChannels     = "pulseboard.alert.channels"
)

// DiscoveredContainer holds the result of discovering a single container.
type DiscoveredContainer struct {
	Container *cmodel.Container
	Err       error
}

// DiscoveryResult holds a discovered container along with its raw labels for endpoint extraction.
type DiscoveryResult struct {
	Container *cmodel.Container
	Labels    map[string]string
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
		cm, err := c.inspectAndMap(ctx, dc, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			// Still include with basic info from list
			cm = mapFromList(dc, now)
		}
		containers = append(containers, cm)
	}

	return containers, nil
}

// DiscoverAllWithLabels is like DiscoverAll but also returns raw Docker labels for each container.
func (c *Client) DiscoverAllWithLabels(ctx context.Context) ([]*DiscoveryResult, error) {
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("container list: %w", err)
	}

	now := time.Now()
	results := make([]*DiscoveryResult, 0, len(list))

	for _, dc := range list {
		cm, err := c.inspectAndMap(ctx, dc, now)
		if err != nil {
			c.logger.Warn("failed to inspect container", "docker_id", dc.ID[:12], "error", err)
			cm = mapFromList(dc, now)
		}
		results = append(results, &DiscoveryResult{
			Container: cm,
			Labels:    dc.Labels,
		})
	}

	return results, nil
}

// inspectAndMap calls ContainerInspect and maps the result to our domain model.
func (c *Client) inspectAndMap(ctx context.Context, dc types.Container, now time.Time) (*cmodel.Container, error) {
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

	// Reclassify exited-with-code-0 containers as "completed" (normal termination).
	if info.State != nil && info.State.Status == "exited" && info.State.ExitCode == 0 {
		cm.State = cmodel.StateCompleted
	}

	return cm, nil
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

	// PulseBoard labels
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
