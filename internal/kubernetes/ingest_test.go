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

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

type captureK8sStore struct {
	agentID    string
	namespaces []string
	workloads  []K8sWorkload
	pods       []K8sPod
	nodes      []K8sNode
	events     []K8sEventRef
}

func (c *captureK8sStore) ReplaceNamespacesForAgent(_ context.Context, agentID string, ns []string) error {
	c.agentID = agentID
	c.namespaces = ns
	return nil
}
func (c *captureK8sStore) ReplaceWorkloadsForAgent(_ context.Context, _ string, w []K8sWorkload) error {
	c.workloads = w
	return nil
}
func (c *captureK8sStore) ReplacePodsForAgent(_ context.Context, _ string, p []K8sPod) error {
	c.pods = p
	return nil
}
func (c *captureK8sStore) ReplaceNodesForAgent(_ context.Context, _ string, n []K8sNode) error {
	c.nodes = n
	return nil
}
func (c *captureK8sStore) ReplaceEventsForAgent(_ context.Context, _ string, e []K8sEventRef) error {
	c.events = e
	return nil
}

func TestIngestService_MapsKubernetesSnapshot(t *testing.T) {
	cap := &captureK8sStore{}
	svc := NewIngestService(cap, nil)

	ev := &agentpb.KubernetesTopology{
		Namespaces: []string{"default", "prod"},
		Workloads: []*agentpb.K8SWorkloadMsg{
			{Id: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Images: []string{"nginx"}, ReadyReplicas: 2, DesiredReplicas: 3, Status: "degraded"},
		},
		Pods: []*agentpb.K8SPodMsg{
			{Name: "web-1", Namespace: "default", Status: "Running", RestartCount: 1, NodeName: "n1", PodIp: "10.0.0.1", WorkloadRef: "default/Deployment/web", Containers: []*agentpb.K8SContainerStatusMsg{
				{Name: "app", Image: "nginx", Ready: true, RestartCount: 1, State: "running"},
			}},
		},
		Nodes: []*agentpb.K8SNodeMsg{
			{Name: "n1", Roles: []string{"worker"}, Status: "ready", RunningPods: 7, KubernetesVersion: "v1.30", CapacityCpuMillicores: 4000, CapacityMemoryBytes: 8 << 30, CapacityPods: 110},
		},
	}

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "kube-A", ev))

	require.Equal(t, "kube-A", cap.agentID)
	require.Equal(t, []string{"default", "prod"}, cap.namespaces)

	require.Len(t, cap.workloads, 1)
	require.Equal(t, "web", cap.workloads[0].Name)
	require.Equal(t, int32(2), cap.workloads[0].ReadyReplicas)
	require.Equal(t, "degraded", cap.workloads[0].Status)

	require.Len(t, cap.pods, 1)
	require.Equal(t, "10.0.0.1", cap.pods[0].PodIP)
	require.Len(t, cap.pods[0].Containers, 1)
	require.True(t, cap.pods[0].Containers[0].Ready)

	require.Len(t, cap.nodes, 1)
	require.Equal(t, []string{"worker"}, cap.nodes[0].Roles)
	require.Equal(t, int64(4000), cap.nodes[0].Capacity.CPUMillicores)
	require.Equal(t, 7, cap.nodes[0].RunningPods)
}

func TestIngestService_NilKubernetesEventIsNoop(t *testing.T) {
	cap := &captureK8sStore{}
	svc := NewIngestService(cap, nil)
	require.NoError(t, svc.HandleAgentEvent(context.Background(), "kube-A", nil))
	require.Nil(t, cap.workloads)
}

func TestIngestService_BroadcastsKubernetesTopologyChanged(t *testing.T) {
	cap := &captureK8sStore{}
	svc := NewIngestService(cap, nil)

	var gotType, gotAgent string
	svc.SetBroadcaster(func(eventType string, data any) {
		gotType = eventType
		if m, ok := data.(map[string]any); ok {
			gotAgent, _ = m["agent_id"].(string)
		}
	})

	require.NoError(t, svc.HandleAgentEvent(context.Background(), "kube-A", &agentpb.KubernetesTopology{}))
	require.Equal(t, "kubernetes.topology_changed", gotType)
	require.Equal(t, "kube-A", gotAgent)
}
