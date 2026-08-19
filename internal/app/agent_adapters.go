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
