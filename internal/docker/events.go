// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"

	"github.com/kolapsis/maintenant/internal/retry"
)

// ContainerEvent represents a processed Docker container/service/node event.
type ContainerEvent struct {
	Action       string
	ExternalID   string
	Name         string
	Image        string
	ExitCode     string
	OOMKilled    bool
	HealthStatus string
	ResourceType string // "container", "service", or "node"
	Timestamp    time.Time
	Labels       map[string]string
}

// StreamEvents relays Docker container, service and node events, resumes a cut stream while the daemon answers, and closes the channel once ctx ends or the daemon is lost.
func (c *Client) StreamEvents(ctx context.Context) <-chan ContainerEvent {
	out := make(chan ContainerEvent, 64)
	go func() {
		defer close(out)
		err := c.relayEvents(ctx, out)
		if ctx.Err() == nil {
			c.SetDisconnected()
			c.logger.Warn("Docker daemon lost, event stream closed", "error", err)
		}
	}()
	return out
}

// relayEvents runs until ctx ends or the daemon stops answering after a cut, and returns why.
func (c *Client) relayEvents(ctx context.Context, out chan<- ContainerEvent) error {
	var since string
	backoff := retry.New(initialBackoff, maxBackoff, 0)
	for {
		resume, cut := c.relaySubscription(ctx, out, since)
		if resume != since {
			since = resume
			backoff.Reset()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.ping(ctx); err != nil {
			return fmt.Errorf("event stream ended (%v) and the daemon does not answer: %w", cut, err)
		}
		delay := backoff.Next()
		c.logger.Info("Docker event stream cut, resubscribing", "error", cut, "retry_in", delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// relaySubscription forwards one subscription's events from since, and returns where to resume and why it ended.
func (c *Client) relaySubscription(ctx context.Context, out chan<- ContainerEvent, since string) (string, error) {
	stream := c.cli.Events(ctx, client.EventsListOptions{
		Since: since,
		Filters: make(client.Filters).Add("type",
			string(events.ContainerEventType),
			string(events.ServiceEventType),
			string(events.NodeEventType),
		),
	})
	for {
		select {
		case <-ctx.Done():
			return since, ctx.Err()
		case err := <-stream.Err:
			return since, err
		case msg, ok := <-stream.Messages:
			if !ok {
				return since, io.EOF
			}
			since = resumeAfter(msg.Time, msg.TimeNano)

			evt := processEvent(msg)
			if evt == nil {
				continue
			}
			if evt.ResourceType == "container" {
				evt.Labels = c.containerLabels(ctx, evt.Labels)
			}
			if evt.Action == "die" && evt.ExitCode == "137" {
				evt.OOMKilled = c.oomKilled(ctx, evt.ExternalID)
			}

			select {
			case out <- *evt:
			case <-ctx.Done():
				return since, ctx.Err()
			}
		}
	}
}

func processEvent(msg events.Message) *ContainerEvent {
	action := string(msg.Action)
	resourceType := string(msg.Type)

	// Service and node events (Swarm).
	if resourceType == string(events.ServiceEventType) || resourceType == string(events.NodeEventType) {
		return &ContainerEvent{
			Action:       action,
			ExternalID:   msg.Actor.ID,
			Name:         msg.Actor.Attributes["name"],
			ResourceType: resourceType,
			Timestamp:    eventTimestamp(msg.Time, msg.TimeNano),
			Labels:       msg.Actor.Attributes,
		}
	}

	// Container events.
	switch {
	case action == "start", action == "stop", action == "die",
		action == "kill", action == "pause", action == "unpause",
		action == "destroy":
		// Standard container lifecycle events
	case strings.HasPrefix(action, "health_status"):
		// Health check events: "health_status: healthy", "health_status: unhealthy"
	default:
		return nil
	}

	evt := &ContainerEvent{
		Action:       action,
		ExternalID:   msg.Actor.ID,
		Name:         msg.Actor.Attributes["name"],
		Image:        msg.Actor.Attributes["image"],
		ResourceType: "container",
		Timestamp:    eventTimestamp(msg.Time, msg.TimeNano),
		Labels:       msg.Actor.Attributes,
	}

	// Extract exit code from die events
	if action == "die" {
		evt.ExitCode = msg.Actor.Attributes["exitCode"]
	}

	// Extract health status from health events
	if strings.HasPrefix(action, "health_status") {
		// Format: "health_status: healthy"
		parts := strings.SplitN(action, ": ", 2)
		if len(parts) == 2 {
			evt.HealthStatus = parts[1]
			evt.Action = "health_status"
		}
	}

	return evt
}

// oomKilled reports whether the kernel OOM killer stopped the container; the die event itself does not say.
func (c *Client) oomKilled(ctx context.Context, id string) bool {
	res, err := c.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		c.logger.Debug("inspect after exit 137 failed, counted as a stop", "docker_id", id, "error", err)
		return false
	}
	return res.Container.State != nil && res.Container.State.OOMKilled
}

// eventTimestamp converts Docker event time fields to time.Time.
// msg.TimeNano contains the full timestamp in nanoseconds (not an offset),
// so we must use it directly or fall back to seconds.
func eventTimestamp(sec int64, nano int64) time.Time {
	if nano > 0 {
		return time.Unix(0, nano)
	}
	return time.Unix(sec, 0)
}

// resumeAfter is the Since value that resubscribes right after the event at sec, nano.
func resumeAfter(sec int64, nano int64) string {
	return eventTimestamp(sec, nano).Add(time.Nanosecond).Format(time.RFC3339Nano)
}
