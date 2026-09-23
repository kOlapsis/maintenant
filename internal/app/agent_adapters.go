// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/kolapsis/maintenant/internal/store"
)

// agentRuntimeResolver adapts the agent store to container.AgentRuntimeResolver,
// so containers reported by a remote agent are tagged with that agent's runtime.
type agentRuntimeResolver struct {
	store *store.AgentStore
}

func (r agentRuntimeResolver) DetectedRuntime(ctx context.Context, agentID string) (string, error) {
	a, err := r.store.Get(ctx, agentID)
	if err != nil {
		return "", err
	}
	return a.DetectedRuntime, nil
}
