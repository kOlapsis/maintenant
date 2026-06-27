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
	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcpK8sStore struct {
	namespaces  []string
	groups      []kubernetes.K8sWorkloadGroup
	pods        []kubernetes.K8sPod
	nodes       []kubernetes.K8sNode
	lastAgentID string
	lastFilters kubernetes.PodFilters
}

func (m *mcpK8sStore) ListNamespaces(_ context.Context, agentID string) ([]string, error) {
	m.lastAgentID = agentID
	return m.namespaces, nil
}
func (m *mcpK8sStore) ListWorkloads(_ context.Context, agentID string, _ []string) ([]kubernetes.K8sWorkloadGroup, error) {
	m.lastAgentID = agentID
	return m.groups, nil
}
func (m *mcpK8sStore) ListPods(_ context.Context, agentID string, _ []string, filters kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	m.lastAgentID = agentID
	m.lastFilters = filters
	return m.pods, nil
}
func (m *mcpK8sStore) ListNodes(_ context.Context, agentID string) ([]kubernetes.K8sNode, error) {
	m.lastAgentID = agentID
	return m.nodes, nil
}

func TestListKubernetesNamespaces(t *testing.T) {
	store := &mcpK8sStore{namespaces: []string{"default", "kube-system"}}
	svc := &Services{Kubernetes: store, Logger: slog.Default(), Version: "test"}

	result, _, err := listKubernetesNamespacesHandler(svc)(context.Background(), nil, listKubernetesNamespacesInput{AgentID: "local"})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, uid.LocalAgent, store.lastAgentID, "'local' must resolve to the LocalAgent sentinel")
	assert.Contains(t, textFromContent(t, result.Content), "kube-system")
}

func TestListKubernetesPods_Filters(t *testing.T) {
	store := &mcpK8sStore{}
	svc := &Services{Kubernetes: store, Logger: slog.Default(), Version: "test"}

	_, _, err := listKubernetesPodsHandler(svc)(context.Background(), nil, listKubernetesPodsInput{
		Workload: "wl-1",
		Node:     "node-a",
		Status:   "Running",
	})
	require.NoError(t, err)
	assert.Equal(t, "wl-1", store.lastFilters.Workload)
	assert.Equal(t, "node-a", store.lastFilters.Node)
	assert.Equal(t, "Running", store.lastFilters.Status)
}

func TestListKubernetesNodes_CE_EditionRequired(t *testing.T) {
	withEdition(t, extension.Community)
	svc := &Services{Kubernetes: &mcpK8sStore{}, Logger: slog.Default(), Version: "test"}

	result, _, err := listKubernetesNodesHandler(svc)(context.Background(), nil, listKubernetesNodesInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "edition_required")
}

func TestListKubernetesNodes_Pro_Success(t *testing.T) {
	withEdition(t, extension.Pro)
	store := &mcpK8sStore{nodes: []kubernetes.K8sNode{{Name: "node-a"}}}
	svc := &Services{Kubernetes: store, Logger: slog.Default(), Version: "test"}

	result, _, err := listKubernetesNodesHandler(svc)(context.Background(), nil, listKubernetesNodesInput{})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "node-a")
}

func TestKubernetes_NilStore(t *testing.T) {
	svc := &Services{Logger: slog.Default(), Version: "test"}
	result, _, err := listKubernetesNamespacesHandler(svc)(context.Background(), nil, listKubernetesNamespacesInput{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textFromContent(t, result.Content), "not available")
}
