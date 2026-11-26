// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/uid"
)

// HandleAgentEvent records an endpoint probe result pushed by a remote agent.
//
// The endpoint is provisioned server-side from the agent container's labels
// (SyncAgentEndpoints), so the result is attached by resolving (agent_id, target)
// — the agent reports its probe target in EndpointEvent.url. If no matching
// endpoint exists yet (a result that raced ahead of the container label sync),
// the event is dropped; the next probe lands once the endpoint is provisioned.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.EndpointEvent, meta agentevent.Meta) error {
	target := ev.GetUrl()
	if target == "" {
		return nil
	}
	ep, err := s.store.GetActiveAgentEndpointByTarget(ctx, agentID, target)
	if err != nil || ep == nil {
		return err
	}
	if _, stopped := s.agentStopped.Load(agentContainerKey{agentID, ep.ExternalID}); stopped {
		return nil
	}

	statusCode := int(ev.GetStatusCode())
	// Degraded is a success: the agent reached the host, only its certificate
	// is untrusted. Collapsing it into the boolean would report it as down.
	degraded := ev.GetStatus() == agentpb.EndpointStatus_ENDPOINT_STATUS_DEGRADED
	result := CheckResult{
		EndpointID:     ep.ID,
		Success:        ev.GetStatus() == agentpb.EndpointStatus_ENDPOINT_STATUS_UP || degraded,
		Degraded:       degraded,
		DegradedReason: ev.GetErrorMessage(),
		ResponseTimeMs: int64(ev.GetLatencyMs()), // #nosec G115 -- probe latency in ms, never approaches int64
		HTTPStatus:     &statusCode,
		ErrorMessage:   ev.GetErrorMessage(),
		Timestamp:      meta.ObservedAt,
		Replayed:       meta.Replayed,
		AgentID:        agentID,
	}
	if meta.EventID != "" {
		result.ID = uid.EventRecord(agentID, meta.EventID, "check_result")
	}
	return s.applyCheckResult(ctx, ep.ID, result)
}

type agentContainerKey struct {
	agentID    string
	externalID string
}

// HandleAgentContainerState sets the endpoints of an agent's container to unknown when it stops, until it runs again.
func (s *Service) HandleAgentContainerState(ctx context.Context, agentID string, ev *agentpb.ContainerEvent) {
	key := agentContainerKey{agentID, ev.GetContainerId()}
	s.agentStopped.Delete(agentAbsentKey(key))
	switch ev.GetState() {
	case agentpb.ContainerState_CONTAINER_STATE_RUNNING:
		s.agentStopped.Delete(key)
		return
	case agentpb.ContainerState_CONTAINER_STATE_EXITED, agentpb.ContainerState_CONTAINER_STATE_DEAD:
	default:
		return
	}
	if ev.GetDestroyed() {
		s.agentStopped.Delete(key)
		return
	}
	if _, known := s.agentStopped.LoadOrStore(key, struct{}{}); known {
		return
	}

	endpoints, err := s.store.ListEndpointsByExternalID(ctx, agentID, key.externalID)
	if err != nil {
		s.agentStopped.Delete(key)
		s.logger.Error("list endpoints for agent container stop", "external_id", key.externalID, "agent_id", agentID, "error", err)
		return
	}
	for _, ep := range endpoints {
		s.setContainerStopped(ctx, ep)
	}
}

// agentAbsentKey marks, in agentStopped, a container that the last inventory of its agent did not list.
type agentAbsentKey agentContainerKey

// RetireAgentContainers deactivates the label endpoints of the agent's containers that two inventories in a row did not list.
func (s *Service) RetireAgentContainers(ctx context.Context, agentID string, live map[string]struct{}) {
	endpoints, err := s.store.ListEndpoints(ctx, ListEndpointsOpts{Source: string(SourceLabel), AgentFilter: &agentID})
	if err != nil {
		s.logger.Error("list agent endpoints for inventory", "agent_id", agentID, "error", err)
		return
	}
	gone := make(map[string]string)
	for _, ep := range endpoints {
		if _, ok := live[ep.ExternalID]; !ok && ep.ExternalID != "" {
			gone[ep.ExternalID] = ep.ContainerName
		}
	}
	for externalID, containerName := range gone {
		key := agentContainerKey{agentID, externalID}
		// One inventory can be older than the event that created the container.
		if _, absent := s.agentStopped.LoadOrStore(agentAbsentKey(key), struct{}{}); !absent {
			continue
		}
		s.agentStopped.Delete(agentAbsentKey(key))
		s.agentStopped.Delete(key)
		s.SyncAgentEndpoints(ctx, agentID, containerName, externalID, nil)
	}
}
