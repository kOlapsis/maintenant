// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"errors"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

type nodeClient struct {
	ServiceClient
	nodes []swarm.Node
	err   error
}

func (c *nodeClient) NodeList(context.Context) ([]swarm.Node, error) { return c.nodes, c.err }
func (c *nodeClient) TaskList(context.Context) ([]swarm.Task, error) { return nil, nil }

type memNodeStore struct{ nodes map[string]*SwarmNode }

func (s *memNodeStore) UpsertNode(_ context.Context, n *SwarmNode) error {
	s.nodes[n.NodeID] = n
	return nil
}
func (s *memNodeStore) ListNodes(context.Context, string) ([]*SwarmNode, error) { return nil, nil }
func (s *memNodeStore) GetNodeByNodeID(_ context.Context, id string) (*SwarmNode, error) {
	return s.nodes[id], nil
}
func (s *memNodeStore) UpdateNodeStatus(context.Context, string, string, string) error { return nil }
func (s *memNodeStore) UpdateNodeTaskCount(context.Context, string, int) error         { return nil }

func managerNode(id string, state swarm.NodeState) swarm.Node {
	return swarm.Node{
		ID:          id,
		Spec:        swarm.NodeSpec{Role: swarm.NodeRoleManager, Availability: swarm.NodeAvailabilityActive},
		Status:      swarm.NodeStatus{State: state},
		Description: swarm.NodeDescription{Hostname: id},
	}
}

func TestNodeService_ALeaderlessSwarmRaisesTheQuorumAlert(t *testing.T) {
	ctx := context.Background()
	client := &nodeClient{err: errors.New("node list: Error response from daemon: rpc error: code = Unknown desc = The swarm does not have a leader. " +
		"It's possible that too few managers are online. Make sure more than half of the managers are online.")}
	sink := &alertSink{}
	ns := NewNodeService(client, &memNodeStore{nodes: map[string]*SwarmNode{}}, quietLogger())
	ns.SetAlertCallback(sink.record)

	require.Error(t, ns.Reconcile(ctx))
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "quorum_degraded", fired[0].AlertType)
	assert.False(t, fired[0].IsRecover)

	require.Error(t, ns.Reconcile(ctx))
	assert.Empty(t, sink.take(), "a lost quorum alerts once")

	client.err = nil
	client.nodes = []swarm.Node{managerNode("m1", swarm.NodeStateReady), managerNode("m2", swarm.NodeStateReady), managerNode("m3", swarm.NodeStateDown)}
	require.NoError(t, ns.Reconcile(ctx))
	recovered := sink.take()
	require.Len(t, recovered, 1)
	assert.True(t, recovered[0].IsRecover)
}

func TestNodeService_OtherNodeListErrorsRaiseNoQuorumAlert(t *testing.T) {
	for _, err := range []error{
		errors.New("node list: Error response from daemon: This node is not a swarm manager. Worker nodes can't be used to view or modify cluster state. " +
			"Please run this command on a manager node or promote the current node to a manager."),
		errors.New("node list: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
		context.DeadlineExceeded,
	} {
		sink := &alertSink{}
		ns := NewNodeService(&nodeClient{err: err}, &memNodeStore{nodes: map[string]*SwarmNode{}}, quietLogger())
		ns.SetAlertCallback(sink.record)

		require.Error(t, ns.Reconcile(context.Background()))
		assert.Empty(t, sink.take(), err.Error())
	}
}

func TestNodeService_AnnouncesNodesThatJoinOrChangeRole(t *testing.T) {
	ctx := context.Background()
	worker := swarm.Node{
		ID:          "n1",
		Spec:        swarm.NodeSpec{Role: swarm.NodeRoleWorker, Availability: swarm.NodeAvailabilityActive},
		Status:      swarm.NodeStatus{State: swarm.NodeStateReady, Addr: "10.0.0.2"},
		Description: swarm.NodeDescription{Hostname: "node-1"},
	}
	client := &nodeClient{nodes: []swarm.Node{worker}}
	var updated []map[string]interface{}
	var statusChanged int
	ns := NewNodeService(client, &memNodeStore{nodes: map[string]*SwarmNode{}}, quietLogger())
	ns.SetEventCallback(func(eventType string, data interface{}) {
		switch eventType {
		case event.SwarmNodeUpdated:
			updated = append(updated, data.(map[string]interface{}))
		case event.SwarmNodeStatusChanged:
			statusChanged++
		}
	})

	require.NoError(t, ns.Reconcile(ctx))
	require.Len(t, updated, 1, "a node that joins is announced")
	assert.Equal(t, "n1", updated[0]["node_id"])
	assert.Equal(t, "worker", updated[0]["role"])

	require.NoError(t, ns.Reconcile(ctx))
	assert.Len(t, updated, 1, "an unchanged node is not announced again")

	client.nodes[0].Spec.Role = swarm.NodeRoleManager
	require.NoError(t, ns.Reconcile(ctx))
	require.Len(t, updated, 2)
	assert.Equal(t, "manager", updated[1]["role"])

	client.nodes[0].Status.State = swarm.NodeStateDown
	require.NoError(t, ns.Reconcile(ctx))
	assert.Len(t, updated, 2, "a status change has its own event")
	assert.Equal(t, 1, statusChanged)
}
