// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package docker

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

// HealthInfo holds parsed health check information from a container.
type HealthInfo struct {
	HasHealthCheck bool
	Status         *cmodel.HealthStatus
}

// GetHealthInfo inspects a container and returns its health check configuration and status.
func (c *Client) GetHealthInfo(ctx context.Context, containerID string) (*HealthInfo, error) {
	res, err := c.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", containerID[:12], err)
	}
	info := res.Container

	hi := &HealthInfo{}

	// Check if container has a HEALTHCHECK defined
	if info.Config != nil && info.Config.Healthcheck != nil && len(info.Config.Healthcheck.Test) > 0 {
		hi.HasHealthCheck = true
	}

	// Get current health status
	if info.State != nil && info.State.Health != nil {
		hs := mapHealthStatus(string(info.State.Health.Status))
		hi.Status = &hs
	}

	return hi, nil
}
