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

package heartbeat

import (
	"context"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// HandleAgentEvent processes a heartbeat ping forwarded by a remote agent.
// The agent_id tracks which agent relayed the ping, but the heartbeat monitor
// is identified by its UUID token (heartbeat_token in the proto).
func (s *Service) HandleAgentEvent(ctx context.Context, agentID string, ev *agentpb.HeartbeatEvent) error {
	token := ev.GetHeartbeatToken()
	if token == "" {
		return nil
	}
	sourceIP := ev.GetSourceIp()
	_, err := s.ProcessPing(ctx, token, sourceIP, "GET", nil, &agentID)
	return err
}
