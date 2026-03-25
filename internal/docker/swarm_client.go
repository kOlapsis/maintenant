// Copyright 2026 Benjamin Touchard (kOlapsis)
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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
)

// SwarmInspect returns the Swarm cluster metadata.
func (c *Client) SwarmInspect(ctx context.Context) (swarm.Swarm, error) {
	sw, err := c.cli.SwarmInspect(ctx)
	if err != nil {
		return swarm.Swarm{}, fmt.Errorf("swarm inspect: %w", err)
	}
	return sw, nil
}

// Info returns the Docker system info (includes Swarm state).
func (c *Client) Info(ctx context.Context) (system.Info, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return system.Info{}, fmt.Errorf("docker info: %w", err)
	}
	return info, nil
}

// ServiceList returns all Swarm services.
func (c *Client) ServiceList(ctx context.Context) ([]swarm.Service, error) {
	services, err := c.cli.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	return services, nil
}

// ServiceInspect returns a single Swarm service with raw data.
func (c *Client) ServiceInspect(ctx context.Context, serviceID string) (swarm.Service, error) {
	svc, _, err := c.cli.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
	if err != nil {
		return swarm.Service{}, fmt.Errorf("service inspect %s: %w", serviceID, err)
	}
	return svc, nil
}

// NodeList returns all Swarm nodes.
func (c *Client) NodeList(ctx context.Context) ([]swarm.Node, error) {
	nodes, err := c.cli.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("node list: %w", err)
	}
	return nodes, nil
}

// TaskList returns all Swarm tasks.
func (c *Client) TaskList(ctx context.Context) ([]swarm.Task, error) {
	tasks, err := c.cli.TaskList(ctx, types.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("task list: %w", err)
	}
	return tasks, nil
}

// NetworkInspect returns details for a network by ID.
func (c *Client) NetworkInspect(ctx context.Context, networkID string) (network.Inspect, error) {
	net, err := c.cli.NetworkInspect(ctx, networkID, network.InspectOptions{})
	if err != nil {
		return network.Inspect{}, fmt.Errorf("network inspect %s: %w", networkID, err)
	}
	return net, nil
}
