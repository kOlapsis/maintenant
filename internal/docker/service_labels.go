// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"time"

	"github.com/moby/moby/client"
)

const (
	labelSwarmServiceID = "com.docker.swarm.service.id"

	serviceLabelsTTL  = 30 * time.Second
	serviceLabelsIdle = 10 * time.Minute
)

type serviceLabels struct {
	labels    map[string]string
	fetchedAt time.Time
}

// swarmServiceLabels returns the labels of a Swarm service, the deploy.labels
// of a stack, cached for serviceLabelsTTL. A node that cannot read services, a
// worker, gets none; a failed refresh keeps the labels last read.
func (c *Client) swarmServiceLabels(ctx context.Context, serviceID string) map[string]string {
	if serviceID == "" {
		return nil
	}
	now := time.Now()

	c.serviceMu.Lock()
	cached, ok := c.serviceLabels[serviceID]
	c.serviceMu.Unlock()
	if ok && now.Sub(cached.fetchedAt) < serviceLabelsTTL {
		return cached.labels
	}

	labels := cached.labels
	res, err := c.cli.ServiceInspect(ctx, serviceID, client.ServiceInspectOptions{})
	if err != nil {
		c.logger.Debug("swarm service labels unavailable", "service_id", serviceID, "error", err)
	} else {
		labels = res.Service.Spec.Labels
	}

	c.serviceMu.Lock()
	defer c.serviceMu.Unlock()
	if c.serviceLabels == nil {
		c.serviceLabels = make(map[string]serviceLabels)
	}
	for id, e := range c.serviceLabels {
		if now.Sub(e.fetchedAt) > serviceLabelsIdle {
			delete(c.serviceLabels, id)
		}
	}
	c.serviceLabels[serviceID] = serviceLabels{labels: labels, fetchedAt: now}
	return labels
}

// withServiceLabels lays a container's labels over those of its service: on a
// key both set, the container's value wins.
func withServiceLabels(labels, service map[string]string) map[string]string {
	if len(service) == 0 {
		return labels
	}
	merged := make(map[string]string, len(service)+len(labels))
	for k, v := range service {
		merged[k] = v
	}
	for k, v := range labels {
		merged[k] = v
	}
	return merged
}
