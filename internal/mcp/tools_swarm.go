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

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSwarmTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_swarm_info",
		Description: "Get Docker Swarm cluster info (manager/worker counts, whether this node is a manager). Reports inactive when Swarm is not detected.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getSwarmInfoHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_swarm_services",
		Description: "List Docker Swarm services with their image, mode, and desired/running replica counts.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listSwarmServicesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_swarm_tasks",
		Description: "List Docker Swarm tasks (the running units of a service), optionally filtered by service, with their state and node.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listSwarmTasksHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_swarm_nodes",
		Description: "List Docker Swarm nodes with their role, status, availability and task count.",
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
