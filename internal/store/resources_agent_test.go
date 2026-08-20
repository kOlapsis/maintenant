// Copyright 2026 Benjamin Touchard (kOlapsis)
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
	"github.com/kolapsis/maintenant/internal/resource"
)

// seedHostContainer inserts a container owned by agentID ("" => local server).
func seedHostContainer(t *testing.T, cstore *ContainerStore, extID string, agentID string) string {
	t.Helper()
	now := time.Now()
	c := &container.Container{
		ExternalID:        extID,
		AgentID:           agentID,
		Name:              extID,
		Image:             "img:v1",
		State:             container.StateRunning,
		AlertSeverity:     container.SeverityWarning,
		RestartThreshold:  3,
		RuntimeType:       "docker",
		FirstSeenAt:       now,
		LastStateChangeAt: now,
	}
	id, err := cstore.InsertContainer(context.Background(), c)
	require.NoError(t, err)
	return id
}

// InsertSnapshot must persist the agent_id column so resource history can be
// scoped per host. The column was previously dropped on every insert.
func TestInsertSnapshot_PersistsAgentID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	agentID := "agent-snap"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: agentID, PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))
	cid := seedHostContainer(t, cstore, "ext-snap", agentID)

	id, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: cid, CPUPercent: 12.5, MemUsed: 100, MemLimit: 200,
		Timestamp: time.Now(), AgentID: agentID,
	})
	require.NoError(t, err)

	var got string
	require.NoError(t, db.Reader().QueryRowContext(ctx,
		`SELECT agent_id FROM resource_snapshots WHERE id = ?`, id).Scan(&got))
	assert.Equal(t, agentID, got)
}

// GetTopConsumersByPeriod must scope by host via the owning container's agent.
func TestGetTopConsumersByPeriod_HostFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	agentStore := NewAgentStore(db)
	cstore := NewContainerStore(db)
	rstore := NewResourceStore(db)

	agentID := "agent-top"
	require.NoError(t, agentStore.Insert(ctx, &agent.Agent{
		AgentID: agentID, PublicKey: make([]byte, 32), Hostname: "h", Label: "edge",
		OSArch: "linux/amd64", AgentVersion: "dev", DetectedRuntime: "docker",
		Status: "active", CreatedAt: time.Now(),
	}))

	localCID := seedHostContainer(t, cstore, "ext-local", "")
	agentCID := seedHostContainer(t, cstore, "ext-agent", agentID)

	now := time.Now()
	_, err := rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: localCID, CPUPercent: 5, MemUsed: 10, MemLimit: 100, Timestamp: now,
	})
	require.NoError(t, err)
	_, err = rstore.InsertSnapshot(ctx, &resource.ResourceSnapshot{
		ContainerID: agentCID, CPUPercent: 80, MemUsed: 90, MemLimit: 100, Timestamp: now, AgentID: agentID,
	})
	require.NoError(t, err)

	ids := func(rows []resource.TopConsumerRow) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.ContainerID
		}
		return out
	}

	// nil => all hosts.
	all, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{localCID, agentCID}, ids(all))

	// "" => local server only.
	local := ""
	localRows, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, &local)
	require.NoError(t, err)
	assert.Equal(t, []string{localCID}, ids(localRows))

	// specific agent.
	agentRows, err := rstore.GetTopConsumersByPeriod(ctx, "cpu", "1h", 10, &agentID)
	require.NoError(t, err)
	assert.Equal(t, []string{agentCID}, ids(agentRows))
}
