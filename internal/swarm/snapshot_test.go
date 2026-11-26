// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"errors"
	"fmt"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotClient struct {
	ServiceClient
	taskCalls int
	nodesErr  error
}

func (c *snapshotClient) ServiceList(context.Context) ([]swarm.Service, error) {
	return []swarm.Service{{ID: "svc1"}, {ID: "svc2"}}, nil
}

func (c *snapshotClient) TaskList(context.Context) ([]swarm.Task, error) {
	c.taskCalls++
	if c.taskCalls > 1 {
		return nil, errors.New("daemon unavailable")
	}
	running := swarm.TaskStatus{State: swarm.TaskStateRunning}
	return []swarm.Task{
		{ID: "t1", ServiceID: "svc1", NodeID: "n1", DesiredState: swarm.TaskStateRunning, Status: running},
		{ID: "t2", ServiceID: "svc2", NodeID: "n1", DesiredState: swarm.TaskStateRunning, Status: running},
		{ID: "t3", ServiceID: "svc2", NodeID: "n1", DesiredState: swarm.TaskStateRemove},
	}, nil
}

func (c *snapshotClient) NodeList(context.Context) ([]swarm.Node, error) {
	if c.nodesErr != nil {
		return nil, c.nodesErr
	}
	return []swarm.Node{{ID: "n1"}}, nil
}

func TestSnapshotFromClient_ListsTasksOnce(t *testing.T) {
	client := &snapshotClient{}

	snap, err := SnapshotFromClient(context.Background(), NewServiceDiscovery(client, quietLogger()), client)
	require.NoError(t, err)

	ids := make([]string, 0, len(snap.Tasks))
	for _, task := range snap.Tasks {
		ids = append(ids, task.TaskID)
	}
	assert.ElementsMatch(t, []string{"t1", "t2"}, ids)
	assert.Equal(t, 1, client.taskCalls)
	require.Len(t, snap.Services, 2)
	assert.Equal(t, 1, snap.Services[0].RunningReplicas)
	require.Len(t, snap.Nodes, 1)
	assert.Equal(t, 2, snap.Nodes[0].TaskCount)
}

func TestSnapshotFromClient_GoesOnWhenNodeListIsRefused(t *testing.T) {
	client := &snapshotClient{nodesErr: fmt.Errorf("list nodes: %w", cerrdefs.ErrPermissionDenied)}

	snap, err := SnapshotFromClient(context.Background(), NewServiceDiscovery(client, quietLogger()), client)
	require.NoError(t, err)
	assert.Len(t, snap.Services, 2)
	assert.Len(t, snap.Tasks, 2)
	assert.Empty(t, snap.Nodes)
}

func TestSnapshotFromClient_FailsOnNodeListError(t *testing.T) {
	client := &snapshotClient{nodesErr: errors.New("daemon unavailable")}

	_, err := SnapshotFromClient(context.Background(), NewServiceDiscovery(client, quietLogger()), client)
	require.Error(t, err)
}
