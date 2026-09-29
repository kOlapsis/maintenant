// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package eol

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/hoststat"
)

// HandleAgentHostOS stores the operating system an agent reports for its host and,
// when it differs from the one already stored, re-evaluates that host.
func (s *Service) HandleAgentHostOS(ctx context.Context, agentID string, ev *agentpb.HostOSMsg) error {
	identity := agent.OSIdentity{
		ID:                ev.GetId(),
		VersionID:         ev.GetVersionId(),
		PrettyName:        ev.GetPrettyName(),
		Source:            sourceFromProto(ev.GetSource()),
		UnavailableReason: ev.GetUnavailableReason(),
	}
	changed, err := s.store.UpdateAgentOS(ctx, agentID, identity, s.now())
	if err != nil {
		return fmt.Errorf("store host os of agent %s: %w", agentID, err)
	}
	if !changed {
		return nil
	}
	if s.onChanged != nil {
		s.onChanged(ctx, agentID)
	}
	return s.EvaluateAgent(ctx, agentID)
}

func sourceFromProto(source agentpb.HostOSSource) string {
	switch source {
	case agentpb.HostOSSource_HOST_OS_SOURCE_HOST_FILE:
		return hoststat.OSSourceHostFile
	case agentpb.HostOSSource_HOST_OS_SOURCE_KUBERNETES_NODE:
		return hoststat.OSSourceKubernetesNode
	default:
		return ""
	}
}
