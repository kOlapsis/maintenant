// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package swarm

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrashLoopDetector_RejectedTasksCountAsFailures(t *testing.T) {
	sink := &alertSink{}
	cld := NewCrashLoopDetector(quietLogger())
	cld.SetAlertCallback(sink.record)

	now := time.Now()
	rejected := func(id string) SwarmTask {
		return SwarmTask{TaskID: id, ServiceID: "svc1", NodeID: "n1", State: "rejected", DesiredState: "shutdown",
			Error: "No such image: registry.example/web:typo", Timestamp: now}
	}
	cld.ObserveTasks(tasksOf(nil, rejected("a"), rejected("b"), rejected("c")))

	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "crash_loop", fired[0].AlertType)
}

func TestNodeService_ANodeFirstSeenDownAlerts(t *testing.T) {
	down := swarm.Node{
		ID:          "n9",
		Spec:        swarm.NodeSpec{Role: swarm.NodeRoleWorker, Availability: swarm.NodeAvailabilityActive},
		Status:      swarm.NodeStatus{State: swarm.NodeStateDown},
		Description: swarm.NodeDescription{Hostname: "worker-9"},
	}
	sink := &alertSink{}
	ns := NewNodeService(&nodeClient{nodes: []swarm.Node{down}}, &memNodeStore{nodes: map[string]*SwarmNode{}}, quietLogger())
	ns.SetAlertCallback(sink.record)

	require.NoError(t, ns.Reconcile(context.Background()))
	fired := sink.take()
	require.Len(t, fired, 1)
	assert.Equal(t, "node_down", fired[0].AlertType)
	assert.Equal(t, "n9", fired[0].EntityID)
	assert.False(t, fired[0].IsRecover)

	require.NoError(t, ns.Reconcile(context.Background()))
	assert.Empty(t, sink.take(), "raised once")
}
