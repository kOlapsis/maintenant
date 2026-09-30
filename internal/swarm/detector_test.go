// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type infoStub struct{ info swarm.Info }

func (s *infoStub) Info(context.Context) (system.Info, error) {
	return system.Info{Swarm: s.info}, nil
}

func TestDetector_ReadsTheClusterOfAManager(t *testing.T) {
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	stub := &infoStub{info: swarm.Info{
		LocalNodeState: swarm.LocalNodeStateActive, ControlAvailable: true, Nodes: 5, Managers: 3,
		Cluster: &swarm.ClusterInfo{ID: "cluster-1", Meta: swarm.Meta{CreatedAt: created}},
	}}
	d := NewDetector(stub, quietLogger())

	result, err := d.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &SwarmCluster{ID: "cluster-1", CreatedAt: created, ManagerCount: 3, WorkerCount: 2, IsManager: true}, result.Cluster())

	stub.info = swarm.Info{LocalNodeState: swarm.LocalNodeStateActive, ControlAvailable: true, Cluster: &swarm.ClusterInfo{}}
	result, err = d.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &SwarmCluster{ID: "cluster-1", CreatedAt: created, ManagerCount: 3, WorkerCount: 2, IsManager: true}, result.Cluster(),
		"a manager without a leader keeps the cluster it read last")
}

func TestDetector_AWorkerManagesNoCluster(t *testing.T) {
	d := NewDetector(&infoStub{info: swarm.Info{LocalNodeState: swarm.LocalNodeStateActive}}, quietLogger())

	result, err := d.Detect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Active)
	assert.Nil(t, result.Cluster())
}

func TestDetector_LogsOnlyRoleChanges(t *testing.T) {
	var logs bytes.Buffer
	manager := swarm.Info{
		LocalNodeState: swarm.LocalNodeStateActive, ControlAvailable: true, Nodes: 3, Managers: 1,
		Cluster: &swarm.ClusterInfo{ID: "cluster-1"},
	}
	stub := &infoStub{info: manager}
	d := NewDetector(stub, slog.New(slog.NewTextHandler(&logs, nil)))
	detect := func(times int) {
		for range times {
			_, err := d.Detect(context.Background())
			require.NoError(t, err)
		}
	}

	detect(3)
	manager.Nodes = 5
	stub.info = manager
	detect(2)
	assert.Equal(t, 1, strings.Count(logs.String(), "manager node"), "a manager staying a manager is logged once")

	stub.info = swarm.Info{LocalNodeState: swarm.LocalNodeStateActive}
	detect(2)
	assert.Equal(t, 1, strings.Count(logs.String(), "worker node"))

	stub.info = swarm.Info{LocalNodeState: swarm.LocalNodeStateInactive}
	detect(2)
	assert.Equal(t, 1, strings.Count(logs.String(), "Swarm mode left"))
}
