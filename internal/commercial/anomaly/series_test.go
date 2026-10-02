// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"testing"

	model "github.com/kolapsis/maintenant/internal/anomaly"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/uid"
)

func TestContainerScopeIDStableAcrossRecreation(t *testing.T) {
	// Same compose project/service, different ephemeral ids/external ids.
	c1 := &container.Container{
		ExternalID: "sha-aaa", Name: "web-1", RuntimeType: "docker",
		OrchestrationGroup: "myproj", OrchestrationUnit: "web",
	}
	c2 := &container.Container{
		ExternalID: "sha-bbb", Name: "web-2", RuntimeType: "docker",
		OrchestrationGroup: "myproj", OrchestrationUnit: "web",
	}
	assert.Equal(t, "compose/myproj/web", ContainerScopeID(c1))
	assert.Equal(t, ContainerScopeID(c1), ContainerScopeID(c2),
		"recreated/scaled compose replicas must share one scope_id")
}

func TestSwarmRebalanceSameService(t *testing.T) {
	// Two tasks of the same swarm service on different nodes / slots.
	c1 := &container.Container{
		Name: "myproj_api.1", RuntimeType: "docker", ControllerKind: "swarm-service",
		SwarmServiceName: "myproj_api", SwarmTaskSlot: 1, SwarmNodeID: "node-a",
	}
	c2 := &container.Container{
		Name: "myproj_api.2", RuntimeType: "docker", ControllerKind: "swarm-service",
		SwarmServiceName: "myproj_api", SwarmTaskSlot: 2, SwarmNodeID: "node-b",
	}
	assert.Equal(t, "swarm/myproj_api", ContainerScopeID(c1))
	assert.Equal(t, ContainerScopeID(c1), ContainerScopeID(c2),
		"swarm replicas rebalanced across nodes keep one service-level scope_id")
}

func TestKubernetesWorkloadScopeID(t *testing.T) {
	dep := &container.Container{
		Name: "web", RuntimeType: "kubernetes", ControllerKind: "Deployment",
		Namespace: "prod", OrchestrationGroup: "prod", OrchestrationUnit: "web",
	}
	ss := &container.Container{
		Name: "db", RuntimeType: "kubernetes", ControllerKind: "StatefulSet",
		Namespace: "prod", OrchestrationGroup: "prod", OrchestrationUnit: "db",
	}
	ds := &container.Container{
		Name: "agent", RuntimeType: "kubernetes", ControllerKind: "DaemonSet",
		Namespace: "kube-system", OrchestrationGroup: "kube-system", OrchestrationUnit: "agent",
	}
	assert.Equal(t, "k8s/prod/Deployment/web", ContainerScopeID(dep))
	assert.Equal(t, "k8s/prod/StatefulSet/db", ContainerScopeID(ss))
	assert.Equal(t, "k8s/kube-system/DaemonSet/agent", ContainerScopeID(ds))

	// A Deployment and a StatefulSet that happen to share a name stay distinct.
	assert.NotEqual(t, ContainerScopeID(dep), ContainerScopeID(&container.Container{
		Name: "web", RuntimeType: "kubernetes", ControllerKind: "StatefulSet",
		Namespace: "prod", OrchestrationGroup: "prod", OrchestrationUnit: "web",
	}))
}

func TestBareContainerScopeIDFallsBackToName(t *testing.T) {
	c := &container.Container{Name: "redis", RuntimeType: "docker"}
	assert.Equal(t, "container/redis", ContainerScopeID(c))
}

func TestHostNodeIDStableAcrossReboot(t *testing.T) {
	// node_id is the agent id, so a host series and its baseline survive a reboot.
	assert.Equal(t, "agent-7", NodeID("agent-7"))
	assert.Equal(t, uid.LocalAgent, NodeID(""), "empty agent id resolves to the local sentinel")
}

func TestContainerHasNoLoadMetric(t *testing.T) {
	assert.False(t, IsMetricValid(model.ScopeTypeContainer, model.MetricLoad),
		"load is a host-only metric; containers must not carry it")
	assert.True(t, IsMetricValid(model.ScopeTypeContainer, model.MetricCPU))
	assert.True(t, IsMetricValid(model.ScopeTypeHost, model.MetricLoad))
	assert.NotContains(t, MetricsForScope(model.ScopeTypeContainer), model.MetricLoad)
	assert.Contains(t, MetricsForScope(model.ScopeTypeHost), model.MetricLoad)
}
