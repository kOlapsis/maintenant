// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import (
	"context"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent processes a heartbeat ping forwarded by a remote agent.
// The agent_id tracks which agent relayed the ping, but the heartbeat monitor
// is identified by its ping token (heartbeat_token in the proto), which is its id.
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.HeartbeatEvent) error {
	token := ev.GetHeartbeatToken()
	if token == "" {
		return nil
	}
	sourceIP := ev.GetSourceIp()
	_, err := s.ProcessPing(ctx, token, sourceIP, "GET", nil, &agentID)
	return err
}
