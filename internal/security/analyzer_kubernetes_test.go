// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeKubernetes_LoadBalancer(t *testing.T) {
	insights := AnalyzeKubernetes("w1", "web", []ServicePort{
		{Service: "shop/web", Type: ServiceTypeLoadBalancer, Port: 443, TargetPort: 8443, Protocol: "tcp"},
	}, testNow)

	require.Len(t, insights, 1)
	assert.Equal(t, ServiceLoadBalancer, insights[0].Type)
	assert.Equal(t, SeverityCritical, insights[0].Severity, "as critical as a Docker port bound on every interface")
	assert.Equal(t, 8443, insights[0].Details["port"])
	assert.Equal(t, "tcp", insights[0].Details["protocol"])
	assert.Equal(t, "shop/web", insights[0].Details["service"])
	assert.Equal(t, "8443/tcp", InsightFindingKey(insights[0]))
}

func TestAnalyzeKubernetes_NodePort(t *testing.T) {
	insights := AnalyzeKubernetes("w1", "web", []ServicePort{
		{Service: "shop/web", Type: ServiceTypeNodePort, Port: 80, NodePort: 30080, Protocol: "tcp"},
	}, testNow)

	require.Len(t, insights, 1)
	assert.Equal(t, ServiceNodePort, insights[0].Type)
	assert.Equal(t, SeverityCritical, insights[0].Severity)
	assert.Equal(t, 80, insights[0].Details["port"], "a named target port falls back on the service port")
	assert.Equal(t, 30080, insights[0].Details["node_port"])
}

func TestAnalyzeKubernetes_DatabasePort(t *testing.T) {
	insights := AnalyzeKubernetes("w1", "db", []ServicePort{
		{Service: "data/pg", Type: ServiceTypeNodePort, Port: 5432, TargetPort: 5432, NodePort: 31432, Protocol: "tcp"},
		{Service: "data/cache", Type: ServiceTypeLoadBalancer, Port: 6380, TargetPort: 6379, Protocol: "tcp"},
	}, testNow)

	require.Len(t, insights, 2)
	for _, i := range insights {
		assert.Equal(t, DatabasePortExposed, i.Type)
		assert.Equal(t, SeverityCritical, i.Severity)
	}
	assert.Equal(t, "PostgreSQL", insights[0].Details["database_type"])
	assert.Equal(t, "Redis", insights[1].Details["database_type"])
}

func TestAnalyzeKubernetes_NoServiceNoInsight(t *testing.T) {
	assert.Empty(t, AnalyzeKubernetes("w1", "web", nil, testNow))
}
