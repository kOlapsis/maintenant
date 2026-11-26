// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fakeSnapshotSource is a static SnapshotSource for exercising
// SnapshotFromRuntime without a live cluster.
type fakeSnapshotSource struct {
	namespaces []string
	pods       []K8sPod
	nodes      []K8sNode
	events     []K8sEventRef
	nodesErr   error
	eventsErr  error
}

func (f fakeSnapshotSource) ListNamespaces(context.Context) ([]string, error) {
	return f.namespaces, nil
}
func (f fakeSnapshotSource) ListWorkloads(context.Context, []string) ([]K8sWorkloadGroup, error) {
	return nil, nil
}
func (f fakeSnapshotSource) ListPods(context.Context, []string, PodFilters) ([]K8sPod, error) {
	return f.pods, nil
}
func (f fakeSnapshotSource) ListNodes(context.Context) ([]K8sNode, error) {
	return f.nodes, f.nodesErr
}
func (f fakeSnapshotSource) ListAllEvents(context.Context) ([]K8sEventRef, error) {
	return f.events, f.eventsErr
}

// TestSnapshotFromRuntime_TalliesRunningPodsPerNode verifies that the snapshot
// derives each node's RunningPods from the pods scheduled on it, since a Node
// object carries no running-pod count.
func TestSnapshotFromRuntime_TalliesRunningPodsPerNode(t *testing.T) {
	src := fakeSnapshotSource{
		namespaces: []string{"demo"},
		pods: []K8sPod{
			{Name: "a", NodeName: "node-1"},
			{Name: "b", NodeName: "node-1"},
			{Name: "c", NodeName: "node-2"},
			{Name: "unscheduled", NodeName: ""}, // not yet placed: counted for no node
		},
		nodes: []K8sNode{
			{Name: "node-1"},
			{Name: "node-2"},
			{Name: "node-3"}, // no pods scheduled
		},
	}

	snap, err := SnapshotFromRuntime(context.Background(), src)
	if err != nil {
		t.Fatalf("SnapshotFromRuntime: %v", err)
	}

	want := map[string]int{"node-1": 2, "node-2": 1, "node-3": 0}
	if len(snap.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(snap.Nodes), len(want))
	}
	for _, n := range snap.Nodes {
		if n.RunningPods != want[n.Name] {
			t.Errorf("node %q: RunningPods = %d, want %d", n.Name, n.RunningPods, want[n.Name])
		}
	}
}

func TestSnapshotFromRuntime_ListErrors(t *testing.T) {
	forbidden := func(resource string) error {
		return k8serrors.NewForbidden(schema.GroupResource{Resource: resource}, "", errors.New("denied"))
	}
	throttled := k8serrors.NewTooManyRequests("slow down", 1)

	tests := []struct {
		name      string
		nodesErr  error
		eventsErr error
		wantErr   bool
	}{
		{name: "nodes forbidden", nodesErr: forbidden("nodes")},
		{name: "events forbidden", eventsErr: forbidden("events")},
		{name: "nodes throttled", nodesErr: throttled, wantErr: true},
		{name: "events throttled", eventsErr: throttled, wantErr: true},
		{name: "nodes timeout", nodesErr: context.DeadlineExceeded, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fakeSnapshotSource{
				namespaces: []string{"demo"},
				pods:       []K8sPod{{Name: "a"}},
				nodesErr:   tt.nodesErr,
				eventsErr:  tt.eventsErr,
			}
			snap, err := SnapshotFromRuntime(context.Background(), src)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(snap.Pods) != 1 {
				t.Fatalf("got %d pods, want 1", len(snap.Pods))
			}
		})
	}
}
