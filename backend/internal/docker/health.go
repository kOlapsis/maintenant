package docker

import (
	"context"
	"fmt"

	cmodel "github.com/kolapsis/pulseboard/internal/container"
)

// HealthInfo holds parsed health check information from a container.
type HealthInfo struct {
	HasHealthCheck bool
	Status         *cmodel.HealthStatus
}

// GetHealthInfo inspects a container and returns its health check configuration and status.
func (c *Client) GetHealthInfo(ctx context.Context, containerID string) (*HealthInfo, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", containerID[:12], err)
	}

	hi := &HealthInfo{}

	// Check if container has a HEALTHCHECK defined
	if info.Config != nil && info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 {
		hi.HasHealthCheck = true
	}

	// Get current health status
	if info.State != nil && info.State.Health != nil {
		hs := mapHealthStatus(info.State.Health.Status)
		hi.Status = &hs
	}

	return hi, nil
}
