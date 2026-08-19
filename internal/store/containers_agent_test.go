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

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/container"
)

// UpdateContainer must persist agent_id. The column was previously missing from
// the SET clause, silently dropping the remote-agent attribution on every update.
func TestUpdateContainer_PersistsAgentID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)

	agentID := "agent-abc"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID:         agentID,
		PublicKey:       make([]byte, 32),
		Hostname:        "host-1",
		Label:           "edge",
		OSArch:          "linux/amd64",
		AgentVersion:    "dev",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       time.Now(),
	}))

	now := time.Now()
	c := &container.Container{
		ExternalID:        "ext-agent-1",
		AgentID:           agentID,
		Name:              "demo",
		Image:             "img:v1",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	}
	id, err := cstore.InsertContainer(ctx, c)
	require.NoError(t, err)
	c.ID = id

	// Mutate an unrelated field and update; agent_id must survive.
	c.Image = "img:v2"
	require.NoError(t, cstore.UpdateContainer(ctx, c))

	got, err := cstore.GetContainerByExternalID(ctx, "ext-agent-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, agentID, got.AgentID, "agent_id must survive UpdateContainer")
	assert.Equal(t, "img:v2", got.Image)
}
