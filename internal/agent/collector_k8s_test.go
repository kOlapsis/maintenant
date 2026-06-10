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

package agent

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/kubernetes"
)

func TestKubernetesTopologyEvent_MarshalsSnapshot(t *testing.T) {
	snap := kubernetes.TopologySnapshot{
		Namespaces: []string{"default", "prod"},
		Workloads: []kubernetes.K8sWorkload{
			{ID: "default/Deployment/web", Name: "web", Namespace: "default", Kind: "Deployment", Images: []string{"nginx"}, ReadyReplicas: 2, DesiredReplicas: 3, Status: "degraded"},
		},
		Pods: []kubernetes.K8sPod{
			{Name: "web-1", Namespace: "default", Status: "Running", RestartCount: 1, NodeName: "n1", PodIP: "10.0.0.1", WorkloadRef: "default/Deployment/web",
				Containers: []kubernetes.K8sContainerStatus{{Name: "app", Image: "nginx", Ready: true, RestartCount: 1, State: "running"}}},
		},
		Nodes: []kubernetes.K8sNode{
			{Name: "n1", Roles: []string{"worker"}, Status: "ready", RunningPods: 7, KubernetesVersion: "v1.30",
				Capacity: kubernetes.K8sResourceQuantity{CPUMillicores: 4000, MemoryBytes: 8 << 30, Pods: 110}},
		},
	}

	ev := kubernetesTopologyEvent("kube-A", snap)

	require.Equal(t, "kube-A", ev.GetAgentId())
	body := ev.GetKubernetes()
	require.NotNil(t, body, "event body must be a kubernetes topology")

	require.Equal(t, []string{"default", "prod"}, body.GetNamespaces())

	require.Len(t, body.GetWorkloads(), 1)
	require.Equal(t, "web", body.GetWorkloads()[0].GetName())
	require.Equal(t, int32(3), body.GetWorkloads()[0].GetDesiredReplicas())

	require.Len(t, body.GetPods(), 1)
	require.Equal(t, "10.0.0.1", body.GetPods()[0].GetPodIp())
	require.Len(t, body.GetPods()[0].GetContainers(), 1)
	require.True(t, body.GetPods()[0].GetContainers()[0].GetReady())

	require.Len(t, body.GetNodes(), 1)
	require.Equal(t, []string{"worker"}, body.GetNodes()[0].GetRoles())
	require.Equal(t, int64(4000), body.GetNodes()[0].GetCapacityCpuMillicores())
	require.Equal(t, int32(7), body.GetNodes()[0].GetRunningPods())
}

// TestCollectKubernetesRuntime_EmitsTopologyAndHostSamples guards the bug fix:
// a Kubernetes agent must report BOTH cluster topology and its own host-level
// resource sample (empty container_id), so the dashboard host gauges populate.
func TestCollectKubernetesRuntime_EmitsTopologyAndHostSamples(t *testing.T) {
	prev := resourceSampleInterval
	resourceSampleInterval = 10 * time.Millisecond
	t.Cleanup(func() { resourceSampleInterval = prev })

	fake := &recordingPushClient{}
	stream := &PushStream{stream: fake, recvCh: make(chan error, 1)}
	id := &Identity{AgentID: "kube-A"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- collectKubernetesRuntime(ctx, id, &fakeSnapshotSource{}, stream, slog.Default())
	}()

	deadline := time.Now().Add(3 * time.Second)
	var sawTopology, sawHostSample bool
	for time.Now().Before(deadline) && !(sawTopology && sawHostSample) {
		for _, ev := range fake.events() {
			if ev.GetKubernetes() != nil {
				sawTopology = true
			}
			if r := ev.GetResource(); r != nil && r.GetContainerId() == "" {
				sawHostSample = true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	require.True(t, sawTopology, "kubernetes agent must push a topology event")
	require.True(t, sawHostSample, "kubernetes agent must push a host-level resource sample (empty container_id)")
}

// fakeSnapshotSource is an empty kubernetes.SnapshotSource: enough for the
// topology pusher to emit one (empty) snapshot without a live cluster.
type fakeSnapshotSource struct{}

func (fakeSnapshotSource) ListNamespaces(context.Context) ([]string, error) { return nil, nil }
func (fakeSnapshotSource) ListWorkloads(context.Context, []string) ([]kubernetes.K8sWorkloadGroup, error) {
	return nil, nil
}
func (fakeSnapshotSource) ListPods(context.Context, []string, kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	return nil, nil
}
func (fakeSnapshotSource) ListNodes(context.Context) ([]kubernetes.K8sNode, error) { return nil, nil }
func (fakeSnapshotSource) ListAllEvents(context.Context) ([]kubernetes.K8sEventRef, error) {
	return nil, nil
}

// recordingPushClient is a minimal agentpb.Ingest_PushClient
// (grpc.BidiStreamingClient[ClientMessage, ServerMessage]) that captures every
// pushed AgentEvent. Send is called concurrently by the topology and host
// goroutines, so the buffer is mutex-guarded.
type recordingPushClient struct {
	mu  sync.Mutex
	buf []*agentpb.AgentEvent
}

func (c *recordingPushClient) Send(m *agentpb.ClientMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ev := m.GetEvent(); ev != nil {
		c.buf = append(c.buf, ev)
	}
	return nil
}

func (c *recordingPushClient) events() []*agentpb.AgentEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*agentpb.AgentEvent(nil), c.buf...)
}

func (c *recordingPushClient) Recv() (*agentpb.ServerMessage, error) { return nil, nil }
func (c *recordingPushClient) Header() (metadata.MD, error)          { return nil, nil }
func (c *recordingPushClient) Trailer() metadata.MD                  { return nil }
func (c *recordingPushClient) CloseSend() error                      { return nil }
func (c *recordingPushClient) Context() context.Context              { return context.Background() }
func (c *recordingPushClient) SendMsg(any) error                     { return nil }
func (c *recordingPushClient) RecvMsg(any) error                     { return nil }
