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
	"strconv"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent records an endpoint check result pushed by a remote agent.
// The EndpointEvent.endpoint_id is the server-assigned integer ID (distributed
// to the agent via AgentConfig). If the endpoint is not found, the event is
// silently dropped (the agent may be running an outdated config).
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.EndpointEvent) error {
	endpointIDStr := ev.GetEndpointId()
	if endpointIDStr == "" {
		return nil
	}
	endpointID, err := strconv.ParseInt(endpointIDStr, 10, 64)
	if err != nil {
		return nil // non-integer IDs are not yet supported
	}

	statusCode := int(ev.GetStatusCode())
	result := CheckResult{
		EndpointID:     endpointID,
		Success:        ev.GetStatus() == agentpb.EndpointStatus_ENDPOINT_STATUS_UP,
		ResponseTimeMs: int64(ev.GetLatencyMs()),
		HTTPStatus:     &statusCode,
		ErrorMessage:   ev.GetErrorMessage(),
		Timestamp:      time.Now(),
		AgentID:        &agentID,
	}
	s.ProcessCheckResult(ctx, endpointID, result)
	return nil
}
