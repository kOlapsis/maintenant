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

package endpoint

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent records an endpoint probe result pushed by a remote agent.
//
// The endpoint is provisioned server-side from the agent container's labels
// (SyncAgentEndpoints), so the result is attached by resolving (agent_id, target)
// — the agent reports its probe target in EndpointEvent.url. If no matching
// endpoint exists yet (a result that raced ahead of the container label sync),
// the event is dropped; the next probe lands once the endpoint is provisioned.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.EndpointEvent) error {
	target := ev.GetUrl()
	if target == "" {
		return nil
	}
	ep, err := s.store.GetActiveAgentEndpointByTarget(ctx, agentID, target)
	if err != nil || ep == nil {
		return err
	}

	statusCode := int(ev.GetStatusCode())
	result := CheckResult{
		EndpointID:     ep.ID,
		Success:        ev.GetStatus() == agentpb.EndpointStatus_ENDPOINT_STATUS_UP,
		ResponseTimeMs: int64(ev.GetLatencyMs()),
		HTTPStatus:     &statusCode,
		ErrorMessage:   ev.GetErrorMessage(),
		Timestamp:      time.Now(),
		AgentID:        agentID,
	}
	s.ProcessCheckResult(ctx, ep.ID, result)
	return nil
}
