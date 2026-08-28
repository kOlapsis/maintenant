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

package kubernetes

import (
	"context"
	"testing"
)

// fakeSnapshotSource is a static SnapshotSource for exercising
// SnapshotFromRuntime without a live cluster.
type fakeSnapshotSource struct {
	namespaces []string
	pods       []K8sPod
	nodes      []K8sNode
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
func (f fakeSnapshotSource) ListNodes(context.Context) ([]K8sNode, error) { return f.nodes, nil }
func (f fakeSnapshotSource) ListAllEvents(context.Context) ([]K8sEventRef, error) {
	return nil, nil
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
