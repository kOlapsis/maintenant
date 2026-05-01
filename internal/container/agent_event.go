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

package container

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent processes a ContainerEvent received from a remote agent.
// It converts the protobuf event to the domain ContainerEvent, routes it
// through ProcessEvent, and ensures the resulting container row has agent_id set.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.ContainerEvent) error {
	action := containerStateToAction(ev.GetState())
	evt := ContainerEvent{
		Action:       action,
		ExternalID:   ev.GetContainerId(),
		Name:         ev.GetName(),
		HealthStatus: "",
		Timestamp:    time.Now(),
		Labels:       ev.GetLabels(),
	}
	if ts := ev.GetStartedAt(); ts != nil {
		evt.Timestamp = ts.AsTime()
	}

	// Route through the normal event pipeline. When ProcessEvent calls
	// InsertContainer, the container.AgentID field is nil — we'll set it below.
	s.ProcessEvent(ctx, evt)

	// Ensure the container record carries the agent attribution.
	// We look up by ExternalID and, if agent_id is not yet set, update it.
	c, err := s.store.GetContainerByExternalID(ctx, ev.GetContainerId())
	if err != nil || c == nil {
		return err
	}
	if c.AgentID == nil {
		c.AgentID = &agentID
		return s.store.UpdateContainer(ctx, c)
	}
	return nil
}

// containerStateToAction maps a proto ContainerState to the action string
// expected by ProcessEvent (mirrors the Docker event action vocabulary).
func containerStateToAction(state agentpb.ContainerState) string {
	switch state {
	case agentpb.ContainerState_CONTAINER_STATE_RUNNING:
		return "start"
	case agentpb.ContainerState_CONTAINER_STATE_EXITED:
		return "die"
	case agentpb.ContainerState_CONTAINER_STATE_PAUSED:
		return "pause"
	case agentpb.ContainerState_CONTAINER_STATE_RESTARTING:
		return "restart"
	case agentpb.ContainerState_CONTAINER_STATE_DEAD:
		return "die"
	case agentpb.ContainerState_CONTAINER_STATE_CREATED:
		return "create"
	default:
		return "start"
	}
}
