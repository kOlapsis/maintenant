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

	got, err := cstore.GetContainerByExternalID(ctx, agentID, "ext-agent-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, agentID, got.AgentID, "agent_id must survive UpdateContainer")
	assert.Equal(t, "img:v2", got.Image)
}

func seedAgent(t *testing.T, db *DB, agentID string) {
	t.Helper()
	require.NoError(t, NewAgentStore(db).Insert(context.Background(), &agent.Agent{
		AgentID:         agentID,
		PublicKey:       make([]byte, 32),
		Hostname:        agentID,
		Label:           agentID,
		OSArch:          "linux/amd64",
		AgentVersion:    "dev",
		DetectedRuntime: "docker",
		Status:          "active",
		CreatedAt:       time.Now(),
	}))
}

func agentContainer(agentID, externalID string) *container.Container {
	now := time.Now()
	return &container.Container{
		ExternalID:        externalID,
		AgentID:           agentID,
		Name:              externalID,
		Image:             "img:v1",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	}
}

func TestContainerStore_GetByExternalID_ScopedByAgent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	seedAgent(t, db, "agent-a")
	seedAgent(t, db, "agent-b")

	const ext = "ext-shared"
	idA, err := cstore.InsertContainer(ctx, agentContainer("agent-a", ext))
	require.NoError(t, err)
	idB, err := cstore.InsertContainer(ctx, agentContainer("agent-b", ext))
	require.NoError(t, err)
	require.NotEqual(t, idA, idB)

	gotA, err := cstore.GetContainerByExternalID(ctx, "agent-a", ext)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	assert.Equal(t, idA, gotA.ID)

	gotB, err := cstore.GetContainerByExternalID(ctx, "agent-b", ext)
	require.NoError(t, err)
	require.NotNil(t, gotB)
	assert.Equal(t, idB, gotB.ID)

	none, err := cstore.GetContainerByExternalID(ctx, "agent-a", "ext-unknown")
	require.NoError(t, err)
	assert.Nil(t, none)
}

func TestContainerStore_ArchiveByID_LeavesSibling(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	seedAgent(t, db, "agent-a")
	seedAgent(t, db, "agent-b")

	const ext = "ext-shared"
	idA, err := cstore.InsertContainer(ctx, agentContainer("agent-a", ext))
	require.NoError(t, err)
	_, err = cstore.InsertContainer(ctx, agentContainer("agent-b", ext))
	require.NoError(t, err)

	require.NoError(t, cstore.ArchiveContainer(ctx, idA, time.Now()))

	gotA, err := cstore.GetContainerByExternalID(ctx, "agent-a", ext)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	assert.True(t, gotA.Archived)

	gotB, err := cstore.GetContainerByExternalID(ctx, "agent-b", ext)
	require.NoError(t, err)
	require.NotNil(t, gotB)
	assert.False(t, gotB.Archived, "archiving by id must not touch the other agent's row")
}

func TestContainerStore_Upsert_ReclaimsAgentID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cstore := NewContainerStore(db)
	seedAgent(t, db, "agent-a")
	seedAgent(t, db, "agent-b")

	const ext = "ext-inherited"
	idA, err := cstore.InsertContainer(ctx, agentContainer("agent-a", ext))
	require.NoError(t, err)

	// The takeover bug left A's row carrying B's agent_id.
	stolen := agentContainer("agent-b", ext)
	stolen.ID = idA
	require.NoError(t, cstore.UpdateContainer(ctx, stolen))
	got, err := cstore.GetContainerByExternalID(ctx, "agent-b", ext)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, idA, got.ID)

	// A's next report reclaims its own row...
	reclaimed, err := cstore.InsertContainer(ctx, agentContainer("agent-a", ext))
	require.NoError(t, err)
	assert.Equal(t, idA, reclaimed)
	gotA, err := cstore.GetContainerByExternalID(ctx, "agent-a", ext)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	assert.Equal(t, idA, gotA.ID)

	// ...and B's next report gets a row of its own.
	idB, err := cstore.InsertContainer(ctx, agentContainer("agent-b", ext))
	require.NoError(t, err)
	assert.NotEqual(t, idA, idB)
	gotB, err := cstore.GetContainerByExternalID(ctx, "agent-b", ext)
	require.NoError(t, err)
	require.NotNil(t, gotB)
	assert.Equal(t, idB, gotB.ID)
}
