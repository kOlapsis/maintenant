// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
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
	HealthStatus string
	ResourceType string // "container", "service", or "node"
	Timestamp    time.Time
	Labels       map[string]string
}

// StreamEvents subscribes to Docker container events and sends them to the returned channel.
// On disconnection, it reconnects with backoff and uses Since to avoid missing events.
// The caller should cancel ctx to stop the stream.
func (c *Client) StreamEvents(ctx context.Context) <-chan ContainerEvent {
	out := make(chan ContainerEvent, 64)

	go func() {
		defer close(out)

		var since string
		backoff := retry.New(initialBackoff, maxBackoff, 0)

		for {
			if err := ctx.Err(); err != nil {
				return
			}

			opts := client.EventsListOptions{
				Filters: make(client.Filters).Add("type",
					string(events.ContainerEventType),
					string(events.ServiceEventType),
					string(events.NodeEventType),
				),
			}
			if since != "" {
				opts.Since = since
			}

			stream := c.cli.Events(ctx, opts)
			msgCh, errCh := stream.Messages, stream.Err

			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-msgCh:
					if !ok {
						goto reconnect
					}
					backoff.Reset() // reset on successful message

					since = timeToSince(msg.Time, msg.TimeNano)

					evt := processEvent(msg)
					if evt == nil {
						continue
					}
					if evt.ResourceType == "container" {
						evt.Labels = c.containerLabels(evt.Labels)
					}

					select {
					case out <- *evt:
					case <-ctx.Done():
						return
					}

				case err, ok := <-errCh:
					if !ok {
						goto reconnect
					}
					c.logger.Warn("Docker event stream error", "error", err)
					c.SetDisconnected()
					goto reconnect
				}
			}

		reconnect:
			delay := backoff.Next()
			c.logger.Info("reconnecting Docker event stream", "backoff", delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}

			// Try to reconnect
			if err := c.Connect(ctx); err != nil {
				c.logger.Warn("Docker reconnect failed", "error", err)
			}
		}
	}()

	return out
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

// eventTimestamp converts Docker event time fields to time.Time.
// msg.TimeNano contains the full timestamp in nanoseconds (not an offset),
// so we must use it directly or fall back to seconds.
func eventTimestamp(sec int64, nano int64) time.Time {
	if nano > 0 {
		return time.Unix(0, nano)
	}
	return time.Unix(sec, 0)
}

func timeToSince(sec int64, nano int64) string {
	if nano > 0 {
		return time.Unix(sec, nano).Format(time.RFC3339Nano)
	}
	return time.Unix(sec, 0).Format(time.RFC3339)
}
