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

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// SwarmInspect returns the Swarm cluster metadata.
func (c *Client) SwarmInspect(ctx context.Context) (swarm.Swarm, error) {
	res, err := c.cli.SwarmInspect(ctx, client.SwarmInspectOptions{})
	if err != nil {
		return swarm.Swarm{}, fmt.Errorf("swarm inspect: %w", err)
	}
	return res.Swarm, nil
}

// Info returns the Docker system info (includes Swarm state).
func (c *Client) Info(ctx context.Context) (system.Info, error) {
	res, err := c.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return system.Info{}, fmt.Errorf("docker info: %w", err)
	}
	return res.Info, nil
}

// ServiceList returns all Swarm services.
func (c *Client) ServiceList(ctx context.Context) ([]swarm.Service, error) {
	res, err := c.cli.ServiceList(ctx, client.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("service list: %w", err)
	}
	return res.Items, nil
}

// ServiceInspect returns a single Swarm service with raw data.
func (c *Client) ServiceInspect(ctx context.Context, serviceID string) (swarm.Service, error) {
	res, err := c.cli.ServiceInspect(ctx, serviceID, client.ServiceInspectOptions{})
	if err != nil {
		return swarm.Service{}, fmt.Errorf("service inspect %s: %w", serviceID, err)
	}
	return res.Service, nil
}

// NodeList returns all Swarm nodes.
func (c *Client) NodeList(ctx context.Context) ([]swarm.Node, error) {
	res, err := c.cli.NodeList(ctx, client.NodeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("node list: %w", err)
	}
	return res.Items, nil
}

// TaskList returns all Swarm tasks.
func (c *Client) TaskList(ctx context.Context) ([]swarm.Task, error) {
	res, err := c.cli.TaskList(ctx, client.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("task list: %w", err)
	}
	return res.Items, nil
}

// NetworkInspect returns details for a network by ID.
func (c *Client) NetworkInspect(ctx context.Context, networkID string) (network.Inspect, error) {
	res, err := c.cli.NetworkInspect(ctx, networkID, client.NetworkInspectOptions{})
	if err != nil {
		return network.Inspect{}, fmt.Errorf("network inspect %s: %w", networkID, err)
	}
	return res.Network, nil
}
