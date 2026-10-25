// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
	addTool(server, svc, &gomcp.Tool{
		Name:        "list_kubernetes_namespaces",
		Description: "List the names of Kubernetes namespaces seen across monitored clusters, deduplicated and sorted, optionally for one cluster agent (agent_id from list_agents). Use them as the namespaces filter of list_kubernetes_workloads and list_kubernetes_pods.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesNamespacesHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_kubernetes_workloads",
		Description: "List Kubernetes workloads (Deployments, StatefulSets, DaemonSets, Jobs) grouped by namespace, each with its ID (namespace/kind/name), images, ready and desired replicas, status (healthy, degraded, progressing or failed) and conditions. Pass an ID as the workload filter of list_kubernetes_pods to see its pods.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesWorkloadsHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_kubernetes_pods",
		Description: "List Kubernetes pods sorted by namespace and name, with status, status reason (e.g. CrashLoopBackOff), restart count, node and per-container state. The namespaces, workload, node and status filters are optional and combine; workload IDs come from list_kubernetes_workloads and node names from list_kubernetes_nodes.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listKubernetesPodsHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_kubernetes_nodes",
		Description: "List Kubernetes nodes with their roles, status (ready, not-ready or unknown), conditions, capacity, allocatable resources, running pod count and versions, optionally for one cluster agent (agent_id from list_agents). Pass a node name as the node filter of list_kubernetes_pods for its pods.",
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
	Workload   string   `json:"workload,omitempty" jsonschema:"Filter by owning workload ID from list_kubernetes_workloads (namespace/kind/name), matched as a prefix"`
	Node       string   `json:"node,omitempty" jsonschema:"Filter by node name, as listed by list_kubernetes_nodes"`
	Status     string   `json:"status,omitempty" jsonschema:"Filter by exact pod phase: Running, Pending, Succeeded, Failed or Unknown"`
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
