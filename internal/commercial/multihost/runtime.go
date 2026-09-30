// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/store"
)

// runtimeRecorder keeps an agent's detected runtime current after enrollment.
type runtimeRecorder struct {
	store       *store.AgentStore
	broadcaster EventBroadcaster
}

// HandleAgentRuntime stores the runtime an agent reports and announces it when it changed.
func (r runtimeRecorder) HandleAgentRuntime(ctx context.Context, agentID string, ev *agentpb.RuntimeMsg) error {
	switch ev.GetKind() {
	case agentpb.Runtime_RUNTIME_DOCKER, agentpb.Runtime_RUNTIME_SWARM, agentpb.Runtime_RUNTIME_KUBERNETES:
	default:
		return fmt.Errorf("agent %s reported an unknown runtime %d", agentID, ev.GetKind())
	}
	runtime := protoRuntimeToString(ev.GetKind())
	changed, err := r.store.UpdateDetectedRuntime(ctx, agentID, runtime)
	if err != nil {
		return err
	}
	if changed {
		r.broadcaster.BroadcastEvent(event.AgentUpdated, map[string]any{
			"agent_id":         agentID,
			"detected_runtime": runtime,
		})
	}
	return nil
}
