// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSwarmTools(server *gomcp.Server, svc *Services) {
	addTool(server, svc, &gomcp.Tool{
		Name:        "get_swarm_info",
		Description: "Get the Docker Swarm cluster the server's own Docker daemon belongs to: cluster_id, created_at, manager_count, worker_count and is_manager. Returns {\"active\": false} when Swarm is not detected. Use list_swarm_nodes, list_swarm_services and list_swarm_tasks for its members.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getSwarmInfoHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_swarm_services",
		Description: "List Docker Swarm services, optionally for one agent (agent_id from list_agents), with service_id, name, image, mode (replicated or global), desired and running replicas, stack, ports and update status. Pass a service_id to list_swarm_tasks to see where its replicas run.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listSwarmServicesHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_swarm_tasks",
		Description: "List Docker Swarm tasks, the containers running a service's replicas, with state, desired state, node, slot, exit code and error. Filter by service_id from list_swarm_services or omit it for all tasks; agent_id (from list_agents) limits them to one agent.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listSwarmTasksHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "list_swarm_nodes",
		Description: "List Docker Swarm nodes with their role (manager or worker), status (ready, down, disconnected or unknown), availability (active, pause or drain), engine version and task count, optionally for one agent (agent_id from list_agents). Use get_swarm_info for the cluster totals.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listSwarmNodesHandler(svc))
}

// --- Input types ---

type getSwarmInfoInput struct{}

type listSwarmServicesInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Filter by agent UUID, or 'local'. Omit for all."`
}

type listSwarmTasksInput struct {
	AgentID   string `json:"agent_id,omitempty" jsonschema:"Filter by agent UUID, or 'local'. Omit for all."`
	ServiceID string `json:"service_id,omitempty" jsonschema:"Filter by service ID. Omit for all tasks."`
}

type listSwarmNodesInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Filter by agent UUID, or 'local'. Omit for all."`
}

// --- Handlers ---

func getSwarmInfoHandler(svc *Services) gomcp.ToolHandlerFor[getSwarmInfoInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ getSwarmInfoInput) (*gomcp.CallToolResult, any, error) {
		if svc.SwarmCluster == nil {
			return jsonResult(map[string]any{"active": false})
		}
		cluster := svc.SwarmCluster()
		if cluster == nil {
			return jsonResult(map[string]any{"active": false})
		}
		return jsonResult(map[string]any{"active": true, "cluster": cluster})
	}
}

func listSwarmServicesHandler(svc *Services) gomcp.ToolHandlerFor[listSwarmServicesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listSwarmServicesInput) (*gomcp.CallToolResult, any, error) {
		if svc.SwarmTopology == nil {
			return errResult("swarm topology store not available")
		}
		services, err := svc.SwarmTopology.ListServices(ctx, resolveAgentScope(input.AgentID))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list swarm services: %w", err)
		}
		return jsonResult(services)
	}
}

func listSwarmTasksHandler(svc *Services) gomcp.ToolHandlerFor[listSwarmTasksInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listSwarmTasksInput) (*gomcp.CallToolResult, any, error) {
		if svc.SwarmTopology == nil {
			return errResult("swarm topology store not available")
		}
		tasks, err := svc.SwarmTopology.ListTasks(ctx, resolveAgentScope(input.AgentID), input.ServiceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list swarm tasks: %w", err)
		}
		return jsonResult(tasks)
	}
}

func listSwarmNodesHandler(svc *Services) gomcp.ToolHandlerFor[listSwarmNodesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listSwarmNodesInput) (*gomcp.CallToolResult, any, error) {
		if svc.SwarmNodes == nil {
			return errResult("swarm node store not available")
		}
		nodes, err := svc.SwarmNodes.ListNodes(ctx, resolveAgentScope(input.AgentID))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list swarm nodes: %w", err)
		}
		return jsonResult(nodes)
	}
}
