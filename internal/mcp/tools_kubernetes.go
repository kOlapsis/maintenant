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
	"fmt"
	"strings"

	"github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/uid"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerKubernetesTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_kubernetes_namespaces",
		Description: "List Kubernetes namespaces known to maintenant across monitored clusters.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesNamespacesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_kubernetes_workloads",
		Description: "List Kubernetes workloads (Deployments, StatefulSets, DaemonSets...) grouped by namespace, with ready/desired replica counts and status.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesWorkloadsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_kubernetes_pods",
		Description: "List Kubernetes pods with their status, restart count and node, optionally filtered by namespace, workload, node or status.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesPodsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_kubernetes_nodes",
		Description: "List Kubernetes nodes with their roles, conditions, capacity and running pod counts.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesNodesHandler(svc))
}

// --- Input types ---

type listKubernetesNamespacesInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Filter by agent/cluster UUID, or 'local'. Omit for all."`
}

type listKubernetesWorkloadsInput struct {
	AgentID    string   `json:"agent_id,omitempty" jsonschema:"Filter by agent/cluster UUID, or 'local'. Omit for all."`
	Namespaces []string `json:"namespaces,omitempty" jsonschema:"Restrict to these namespaces"`
}

type listKubernetesPodsInput struct {
	AgentID    string   `json:"agent_id,omitempty" jsonschema:"Filter by agent/cluster UUID, or 'local'. Omit for all."`
	Namespaces []string `json:"namespaces,omitempty" jsonschema:"Restrict to these namespaces"`
	Workload   string   `json:"workload,omitempty" jsonschema:"Filter by owning workload ID"`
	Node       string   `json:"node,omitempty" jsonschema:"Filter by node name"`
	Status     string   `json:"status,omitempty" jsonschema:"Filter by pod status phase"`
}

type listKubernetesNodesInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Filter by agent/cluster UUID, or 'local'. Omit for all."`
}

// resolveAgentScope maps the MCP agent_id input to a store scope: "local" maps
// to the LocalAgent sentinel, anything else (including "") is passed verbatim,
// where an empty string means "all agents".
func resolveAgentScope(agentID string) string {
	if strings.EqualFold(agentID, "local") {
		return uid.LocalAgent
	}
	return agentID
}

// --- Handlers ---

func listKubernetesNamespacesHandler(svc *Services) gomcp.ToolHandlerFor[listKubernetesNamespacesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listKubernetesNamespacesInput) (*gomcp.CallToolResult, any, error) {
		if svc.Kubernetes == nil {
			return errResult("kubernetes store not available")
		}
		ns, err := svc.Kubernetes.ListNamespaces(ctx, resolveAgentScope(input.AgentID))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
		}
		return jsonResult(ns)
	}
}

func listKubernetesWorkloadsHandler(svc *Services) gomcp.ToolHandlerFor[listKubernetesWorkloadsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listKubernetesWorkloadsInput) (*gomcp.CallToolResult, any, error) {
		if svc.Kubernetes == nil {
			return errResult("kubernetes store not available")
		}
		groups, err := svc.Kubernetes.ListWorkloads(ctx, resolveAgentScope(input.AgentID), input.Namespaces)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list workloads: %w", err)
		}
		return jsonResult(groups)
	}
}

func listKubernetesPodsHandler(svc *Services) gomcp.ToolHandlerFor[listKubernetesPodsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listKubernetesPodsInput) (*gomcp.CallToolResult, any, error) {
		if svc.Kubernetes == nil {
			return errResult("kubernetes store not available")
		}
		filters := kubernetes.PodFilters{Workload: input.Workload, Node: input.Node, Status: input.Status}
		pods, err := svc.Kubernetes.ListPods(ctx, resolveAgentScope(input.AgentID), input.Namespaces, filters)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list pods: %w", err)
		}
		return jsonResult(pods)
	}
}

func listKubernetesNodesHandler(svc *Services) gomcp.ToolHandlerFor[listKubernetesNodesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listKubernetesNodesInput) (*gomcp.CallToolResult, any, error) {
		if svc.Kubernetes == nil {
			return errResult("kubernetes store not available")
		}
		nodes, err := svc.Kubernetes.ListNodes(ctx, resolveAgentScope(input.AgentID))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
		}
		return jsonResult(nodes)
	}
}
