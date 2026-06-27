// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package mcp

import (
	"context"
	"log/slog"
	"testing"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcpSwarmTopology struct {
	services      []*swarm.SwarmService
	tasks         []*swarm.SwarmTask
	lastAgentID   string
	lastServiceID string
}

func (m *mcpSwarmTopology) ListServices(_ context.Context, agentID string) ([]*swarm.SwarmService, error) {
	m.lastAgentID = agentID
	return m.services, nil
}
func (m *mcpSwarmTopology) ListTasks(_ context.Context, agentID, serviceID string) ([]*swarm.SwarmTask, error) {
	m.lastAgentID = agentID
	m.lastServiceID = serviceID
	return m.tasks, nil
}

type mcpSwarmNodes struct {
	nodes []*swarm.SwarmNode
}

func (m *mcpSwarmNodes) ListNodes(_ context.Context, _ string) ([]*swarm.SwarmNode, error) {
	return m.nodes, nil
}

func TestGetSwarmInfo_Inactive(t *testing.T) {
	// Cluster closure returns nil (Swarm not active).
	svc := &Services{SwarmCluster: func() *swarm.SwarmCluster { return nil }, Logger: slog.Default(), Version: "test"}
	result, _, err := getSwarmInfoHandler(svc)(context.Background(), nil, getSwarmInfoInput{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), `"active":false`)
}

func TestGetSwarmInfo_Active(t *testing.T) {
	cluster := &swarm.SwarmCluster{ID: "cluster-1", IsManager: true}
	svc := &Services{SwarmCluster: func() *swarm.SwarmCluster { return cluster }, Logger: slog.Default(), Version: "test"}
	result, _, err := getSwarmInfoHandler(svc)(context.Background(), nil, getSwarmInfoInput{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	text := textFromContent(t, result.Content)
	assert.Contains(t, text, `"active":true`)
	assert.Contains(t, text, "cluster-1")
}

func TestListSwarmServices(t *testing.T) {
	topo := &mcpSwarmTopology{services: []*swarm.SwarmService{{Name: "web"}}}
	svc := &Services{SwarmTopology: topo, Logger: slog.Default(), Version: "test"}
	result, _, err := listSwarmServicesHandler(svc)(context.Background(), nil, listSwarmServicesInput{AgentID: "local"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, uid.LocalAgent, topo.lastAgentID)
	assert.Contains(t, textFromContent(t, result.Content), "web")
}

func TestListSwarmTasks_ServiceFilter(t *testing.T) {
	topo := &mcpSwarmTopology{}
	svc := &Services{SwarmTopology: topo, Logger: slog.Default(), Version: "test"}
	_, _, err := listSwarmTasksHandler(svc)(context.Background(), nil, listSwarmTasksInput{ServiceID: "svc-9"})
	require.NoError(t, err)
	assert.Equal(t, "svc-9", topo.lastServiceID)
}

func TestListSwarmNodes_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{SwarmNodes: &mcpSwarmNodes{}, Logger: slog.Default(), Version: "test"}
	result, _, err := listSwarmNodesHandler(svc)(context.Background(), nil, listSwarmNodesInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestListSwarmNodes_Pro_Success(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{SwarmNodes: &mcpSwarmNodes{nodes: []*swarm.SwarmNode{{Hostname: "mgr-1"}}}, Logger: slog.Default(), Version: "test"}
	result, _, err := listSwarmNodesHandler(svc)(context.Background(), nil, listSwarmNodesInput{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "mgr-1")
}
