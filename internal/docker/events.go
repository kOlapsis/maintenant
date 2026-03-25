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
	"strings"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// ContainerEvent represents a processed Docker container/service/node event.
type ContainerEvent struct {
	Action       string
	ExternalID   string
	Name         string
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
		backoff := initialBackoff

		for {
			if err := ctx.Err(); err != nil {
				return
			}

			opts := events.ListOptions{
				Filters: filters.NewArgs(
					filters.Arg("type", string(events.ContainerEventType)),
				filters.Arg("type", string(events.ServiceEventType)),
				filters.Arg("type", string(events.NodeEventType)),
				),
			}
			if since != "" {
				opts.Since = since
			}

			msgCh, errCh := c.cli.Events(ctx, opts)

			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-msgCh:
					if !ok {
						goto reconnect
					}
					backoff = initialBackoff // reset on successful message

					since = timeToSince(msg.Time, msg.TimeNano)

					evt := processEvent(msg)
					if evt == nil {
						continue
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
			c.logger.Info("reconnecting Docker event stream", "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
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
