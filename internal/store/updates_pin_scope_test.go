// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/kolapsis/maintenant/internal/update"
)

func seedSwarmTask(t *testing.T, cs *ContainerStore, agentID, externalID, serviceID string) string {
	t.Helper()
	now := time.Now()
	id, err := cs.InsertContainer(context.Background(), &container.Container{
		ExternalID: externalID, AgentID: agentID, Name: "shop_web.1." + externalID, Image: "nginx:1.25.3",
		State: container.StateRunning, AlertSeverity: container.SeverityWarning, RestartThreshold: 3, RuntimeType: "docker",
		SwarmServiceID: serviceID, SwarmServiceName: "shop_web", FirstSeenAt: now, LastStateChangeAt: now,
	})
	require.NoError(t, err)
	return id
}

func TestAdoptVersionPin_FollowsTheSwarmServiceAcrossHosts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs, us := NewContainerStore(db), NewUpdateStore(db)
	for _, a := range []string{"agent-a", "agent-b", "agent-other-swarm"} {
		seedAgent(t, db, a)
	}
	adopt := func(agentID, taskID string) *update.VersionPin {
		t.Helper()
		pin, err := us.AdoptVersionPin(ctx, agentID, taskID, "shop_web", "nginx:1.25.3")
		require.NoError(t, err)
		return pin
	}

	firstUID := seedSwarmTask(t, cs, "agent-a", "task-1", "svc-1")
	_, err := us.InsertVersionPin(ctx, &update.VersionPin{
		AgentID: "agent-a", ContainerID: "task-1", ContainerName: "shop_web", Image: "nginx:1.25.3", PinnedTag: "1.25.3", PinnedAt: time.Now(),
	})
	require.NoError(t, err)

	seedSwarmTask(t, cs, "agent-b", "task-2", "svc-1")
	require.NotNil(t, adopt("agent-b", "task-2"), "a replica on another host is pinned with its service")
	require.NotNil(t, adopt("agent-a", "task-1"), "and the pinned task keeps its pin")

	seedSwarmTask(t, cs, "agent-other-swarm", "task-9", "svc-9")
	assert.Nil(t, adopt("agent-other-swarm", "task-9"), "a service of the same name in another swarm is not pinned")

	require.NoError(t, cs.ArchiveContainer(ctx, firstUID, time.Now()))
	seedSwarmTask(t, cs, "agent-b", "task-3", "svc-1")
	require.NotNil(t, adopt("agent-b", "task-3"), "the task rescheduled on another host keeps the pin")

	_, err = cs.DeleteArchivedContainersBefore(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, db.ReadDB(), "version_pins"), "purging the former task leaves the pin of the service")
	require.NotNil(t, adopt("agent-b", "task-2"))
}

func TestAdoptVersionPin_OnlyForTheSameImageRepository(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	cs, us := NewContainerStore(db), NewUpdateStore(db)

	oldUID := seedHostedContainer(t, cs, uid.LocalAgent, "ctr-old", "db", "postgres:14.12")
	_, err := us.InsertVersionPin(ctx, &update.VersionPin{
		ContainerID: "ctr-old", ContainerName: "db", Image: "postgres:14.12", PinnedTag: "14.12", PinnedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, cs.ArchiveContainer(ctx, oldUID, time.Now()))

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-maria", "db", "mariadb:11.4.2")
	pin, err := us.AdoptVersionPin(ctx, "", "ctr-maria", "db", "mariadb:11.4.2")
	require.NoError(t, err)
	assert.Nil(t, pin, "a container of another image does not inherit the pin")

	seedHostedContainer(t, cs, uid.LocalAgent, "ctr-pg", "db", "docker.io/library/postgres:14.13@sha256:abc")
	pin, err = us.AdoptVersionPin(ctx, "", "ctr-pg", "db", "docker.io/library/postgres:14.13@sha256:abc")
	require.NoError(t, err)
	require.NotNil(t, pin, "the same repository under another tag is the same software")
	assert.Equal(t, "ctr-pg", pin.ContainerID)
}
