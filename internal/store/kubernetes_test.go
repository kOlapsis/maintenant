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

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/kubernetes"
)

func TestKubernetesStore_Workloads_ReconcileAndPrune(t *testing.T) {
	db := openTestDB(t)
	store := NewKubernetesStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "kube-A", "kubernetes")
	seedTestAgent(t, db, "kube-B", "kubernetes")

	require.NoError(t, store.ReplaceWorkloadsForAgent(ctx, "kube-A", []kubernetes.K8sWorkload{
		{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Images: []string{"nginx:1"}, ReadyReplicas: 3, DesiredReplicas: 3, Status: "healthy"},
		{ID: "default/Deployment/api", Name: "api", Namespace: "default", Kind: "Deployment", Images: []string{"api:2"}, ReadyReplicas: 0, DesiredReplicas: 2, Status: "failed"},
	}))
	require.NoError(t, store.ReplaceWorkloadsForAgent(ctx, "kube-B", []kubernetes.K8sWorkload{
		{ID: "prod/StatefulSet/db", Name: "db", Namespace: "prod", Kind: "StatefulSet", ReadyReplicas: 1, DesiredReplicas: 1, Status: "healthy"},
	}))

	groups, err := store.ListWorkloads(ctx, "kube-A", nil)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "default", groups[0].Namespace)
	require.Len(t, groups[0].Workloads, 2)
	require.Equal(t, []string{"nginx:1"}, groups[0].Workloads[1].Images) // api sorts before web? ordered by kind,name

	// Second snapshot drops "api" — hard-deleted; B untouched.
	require.NoError(t, store.ReplaceWorkloadsForAgent(ctx, "kube-A", []kubernetes.K8sWorkload{
		{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Images: []string{"nginx:2"}, ReadyReplicas: 5, DesiredReplicas: 5, Status: "healthy"},
	}))
	groups, err = store.ListWorkloads(ctx, "kube-A", nil)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Workloads, 1)
	require.Equal(t, int32(5), groups[0].Workloads[0].DesiredReplicas)
	require.Equal(t, []string{"nginx:2"}, groups[0].Workloads[0].Images)

	bGroups, err := store.ListWorkloads(ctx, "kube-B", nil)
	require.NoError(t, err)
	require.Len(t, bGroups, 1, "kube-B untouched")

	// Namespace filter.
	none, err := store.ListWorkloads(ctx, "kube-A", []string{"other"})
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestKubernetesStore_Pods_JSONRoundtripAndFilters(t *testing.T) {
	db := openTestDB(t)
	store := NewKubernetesStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "kube-A", "kubernetes")

	require.NoError(t, store.ReplacePodsForAgent(ctx, "kube-A", []kubernetes.K8sPod{
		{
			Namespace: "default", Name: "web-1", Status: "Running", RestartCount: 2,
			NodeName: "node-1", PodIP: "10.0.0.1", HostIP: "192.168.0.1", WorkloadRef: "default/Deployment/web",
			Containers: []kubernetes.K8sContainerStatus{{Name: "app", Image: "nginx:1", Ready: true, RestartCount: 2, State: "running"}},
		},
		{
			Namespace: "default", Name: "web-2", Status: "Pending", StatusReason: "ImagePullBackOff",
			NodeName: "node-2", WorkloadRef: "default/Deployment/web",
		},
	}))

	pods, err := store.ListPods(ctx, "kube-A", nil, kubernetes.PodFilters{})
	require.NoError(t, err)
	require.Len(t, pods, 2)
	require.Len(t, pods[0].Containers, 1)
	require.Equal(t, "app", pods[0].Containers[0].Name)
	require.True(t, pods[0].Containers[0].Ready)

	// Node filter.
	onNode2, err := store.ListPods(ctx, "kube-A", nil, kubernetes.PodFilters{Node: "node-2"})
	require.NoError(t, err)
	require.Len(t, onNode2, 1)
	require.Equal(t, "web-2", onNode2[0].Name)

	// Workload prefix filter.
	byWl, err := store.ListPods(ctx, "kube-A", nil, kubernetes.PodFilters{Workload: "default/Deployment/web"})
	require.NoError(t, err)
	require.Len(t, byWl, 2)
}

func TestKubernetesStore_Events_ReplaceAndLookupByObject(t *testing.T) {
	db := openTestDB(t)
	store := NewKubernetesStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "kube-A", "kubernetes")

	require.NoError(t, store.ReplaceEventsForAgent(ctx, "kube-A", []kubernetes.K8sEventRef{
		{
			K8sEvent:     kubernetes.K8sEvent{Type: "Warning", Reason: "BackOff", Message: "crash", Count: 5},
			InvolvedKind: "Pod", InvolvedNamespace: "default", InvolvedName: "web-1",
		},
		{
			K8sEvent:     kubernetes.K8sEvent{Type: "Normal", Reason: "ScalingReplicaSet", Message: "scaled up"},
			InvolvedKind: "Deployment", InvolvedNamespace: "default", InvolvedName: "web",
		},
	}))

	podEvents, err := store.ListEventsForObject(ctx, "kube-A", "Pod", "default", "web-1")
	require.NoError(t, err)
	require.Len(t, podEvents, 1)
	require.Equal(t, "BackOff", podEvents[0].Reason)
	require.Equal(t, int32(5), podEvents[0].Count)

	depEvents, err := store.ListEventsForObject(ctx, "kube-A", "Deployment", "default", "web")
	require.NoError(t, err)
	require.Len(t, depEvents, 1)
	require.Equal(t, "ScalingReplicaSet", depEvents[0].Reason)

	// No events for an unrelated object.
	none, err := store.ListEventsForObject(ctx, "kube-A", "Pod", "default", "other")
	require.NoError(t, err)
	require.Empty(t, none)

	// Wholesale replace supersedes the previous set.
	require.NoError(t, store.ReplaceEventsForAgent(ctx, "kube-A", nil))
	podEvents, err = store.ListEventsForObject(ctx, "kube-A", "Pod", "default", "web-1")
	require.NoError(t, err)
	require.Empty(t, podEvents)
}

func TestKubernetesStore_NamespacesAndNodes(t *testing.T) {
	db := openTestDB(t)
	store := NewKubernetesStore(db)
	ctx := context.Background()
	seedTestAgent(t, db, "kube-A", "kubernetes")

	require.NoError(t, store.ReplaceNamespacesForAgent(ctx, "kube-A", []string{"default", "kube-system", "prod"}))
	ns, err := store.ListNamespaces(ctx, "kube-A")
	require.NoError(t, err)
	require.Equal(t, []string{"default", "kube-system", "prod"}, ns)

	// Wholesale replace prunes removed namespaces.
	require.NoError(t, store.ReplaceNamespacesForAgent(ctx, "kube-A", []string{"default"}))
	ns, err = store.ListNamespaces(ctx, "kube-A")
	require.NoError(t, err)
	require.Equal(t, []string{"default"}, ns)

	require.NoError(t, store.ReplaceNodesForAgent(ctx, "kube-A", []kubernetes.K8sNode{
		{
			Name: "node-1", Roles: []string{"control-plane"}, Status: "ready", RunningPods: 12,
			KubernetesVersion: "v1.30.0", OSImage: "Ubuntu 22.04", Architecture: "amd64",
			Capacity:    kubernetes.K8sResourceQuantity{CPUMillicores: 4000, MemoryBytes: 8 << 30, Pods: 110},
			Allocatable: kubernetes.K8sResourceQuantity{CPUMillicores: 3800, MemoryBytes: 7 << 30, Pods: 110},
		},
	}))
	nodes, err := store.ListNodes(ctx, "kube-A")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, []string{"control-plane"}, nodes[0].Roles)
	require.Equal(t, int64(4000), nodes[0].Capacity.CPUMillicores)
	require.Equal(t, 12, nodes[0].RunningPods)
}
