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

// Package mcp — escalation tools
//
// Tool-to-HTTP mapping:
//
//	list_escalation_policies      → GET    /api/v1/escalation-policies
//	get_escalation_policy         → GET    /api/v1/escalation-policies/{id}
//	create_escalation_policy      → POST   /api/v1/escalation-policies
//	delete_escalation_policy      → DELETE /api/v1/escalation-policies/{id}
//	update_escalation_policy      → PUT    /api/v1/escalation-policies/{id}
//	set_escalation_policy_active  → PATCH  /api/v1/escalation-policies/{id}/active
//	list_alert_escalation_runs    → GET    /api/v1/alerts/{alert_id}/escalation-runs
//	get_escalation_run            → GET    /api/v1/escalation-runs/{id}

package mcp

import (
	"context"
	"fmt"

	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerEscalationTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_escalation_policies",
		Description: "List all escalation policies with their filters and levels. Requires Pro edition.",
	}, listEscalationPoliciesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_escalation_policy",
		Description: "Get a single escalation policy by ID. Requires Pro edition.",
	}, getEscalationPolicyHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_escalation_policy",
		Description: "Create a new escalation policy. Requires Pro edition.",
	}, createEscalationPolicyHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "delete_escalation_policy",
		Description: "Delete an escalation policy and stop any active runs. Requires Pro edition.",
	}, deleteEscalationPolicyHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_alert_escalation_runs",
		Description: "List escalation runs attached to a specific alert. Requires Pro edition.",
	}, listAlertEscalationRunsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_escalation_run",
		Description: "Get full detail of an escalation run including all deliveries. Requires Pro edition.",
	}, getEscalationRunHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_escalation_policy",
		Description: "Update an existing escalation policy (name, filters, levels, active). Requires Pro edition.",
	}, updateEscalationPolicyHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "set_escalation_policy_active",
		Description: "Activate or deactivate an escalation policy. Requires Pro edition.",
	}, setEscalationPolicyActiveHandler(svc))
}

// --- input types ---

type listEscalationPoliciesInput struct {
	ActiveOnly bool `json:"active_only" jsonschema:"Filter to active policies only"`
}

type getEscalationPolicyInput struct {
	ID int64 `json:"id" jsonschema:"Policy ID"`
}

type createEscalationPolicyInput struct {
	Name    string                 `json:"name" jsonschema:"Policy name (1-120 chars)"`
	Active  bool                   `json:"active" jsonschema:"Whether the policy is active"`
	Filters escalationFiltersInput `json:"filters" jsonschema:"Alert matching filters"`
	Levels  []escalationLevelInput `json:"levels" jsonschema:"Escalation levels (min 1, max 5)"`
}

type escalationFiltersInput struct {
	Severities []string               `json:"severities,omitempty" jsonschema:"Severity filters: warning, critical"`
	Scopes     []escalationScopeInput `json:"scopes,omitempty" jsonschema:"Scope filters"`
	Tags       []string               `json:"tags,omitempty" jsonschema:"Tag filters"`
}

type escalationScopeInput struct {
	Kind  string `json:"kind" jsonschema:"Scope kind: container, endpoint, heartbeat, certificate, monitor"`
	RefID int64  `json:"ref_id" jsonschema:"Referenced entity ID"`
}

type escalationLevelInput struct {
	DelaySeconds int     `json:"delay_seconds" jsonschema:"Delay in seconds (60-86400)"`
	ChannelIDs   []int64 `json:"channel_ids" jsonschema:"Notification channel IDs (at least one required)"`
}

type deleteEscalationPolicyInput struct {
	ID int64 `json:"id" jsonschema:"Policy ID to delete"`
}

type listAlertEscalationRunsInput struct {
	AlertID int64 `json:"alert_id" jsonschema:"Alert ID"`
}

type getEscalationRunInput struct {
	ID int64 `json:"id" jsonschema:"Run ID"`
}

type updateEscalationPolicyInput struct {
	ID      int64                  `json:"id" jsonschema:"Policy ID to update"`
	Name    string                 `json:"name" jsonschema:"Policy name (1-120 chars)"`
	Active  bool                   `json:"active" jsonschema:"Whether the policy is active"`
	Filters escalationFiltersInput `json:"filters" jsonschema:"Alert matching filters"`
	Levels  []escalationLevelInput `json:"levels" jsonschema:"Escalation levels (min 1, max 5)"`
}

type setEscalationPolicyActiveInput struct {
	ID     int64 `json:"id" jsonschema:"Policy ID"`
	Active bool  `json:"active" jsonschema:"New active status"`
}

// --- edition check helper ---

func checkEscalationEdition() (*gomcp.CallToolResult, any, error) {
	if extension.CurrentEdition() != extension.Enterprise {
		msg := `{"error":"edition_required","feature":"alert_escalation","message":"This tool requires Maintenant Pro. Upgrade at https://maintenant.dev/pro"}`
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
			IsError: true,
		}, nil, nil
	}
	return nil, nil, nil
}

// --- handlers ---

func listEscalationPoliciesHandler(svc *Services) gomcp.ToolHandlerFor[listEscalationPoliciesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listEscalationPoliciesInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		limits, err := svc.EscalationSvc.GetPlanLimits(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list escalation policies: get limits: %w", err)
		}
		policies, err := svc.EscalationSvc.ListPolicies(ctx, input.ActiveOnly)
		if err != nil {
			return nil, nil, fmt.Errorf("list escalation policies: %w", err)
		}
		return jsonResult(map[string]any{
			"policies": policies,
			"limits":   limits,
		})
	}
}

func getEscalationPolicyHandler(svc *Services) gomcp.ToolHandlerFor[getEscalationPolicyInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getEscalationPolicyInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		policy, err := svc.EscalationSvc.GetPolicy(ctx, input.ID)
		if err != nil {
			return errResult(fmt.Sprintf("policy_not_found: %s", err.Error()))
		}
		return jsonResult(policy)
	}
}

func createEscalationPolicyHandler(svc *Services) gomcp.ToolHandlerFor[createEscalationPolicyInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createEscalationPolicyInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}

		levels := make([]escalation.LevelReq, len(input.Levels))
		for i, l := range input.Levels {
			levels[i] = escalation.LevelReq{
				DelaySeconds: l.DelaySeconds,
				ChannelIDs:   l.ChannelIDs,
			}
		}

		scopes := make([]escalation.Scope, len(input.Filters.Scopes))
		for i, s := range input.Filters.Scopes {
			scopes[i] = escalation.Scope{Kind: s.Kind, RefID: s.RefID}
		}

		req := escalation.PolicyRequest{
			Name:   input.Name,
			Active: input.Active,
			Filters: escalation.Filters{
				Severities: input.Filters.Severities,
				Scopes:     scopes,
				Tags:       input.Filters.Tags,
			},
			Levels: levels,
		}

		policy, err := svc.EscalationSvc.CreatePolicy(ctx, req)
		if err != nil {
			return errResult(fmt.Sprintf("validation_failed: %s", err.Error()))
		}
		return jsonResult(map[string]any{
			"id":                   policy.ID,
			"name":                 policy.Name,
			"active":               policy.Active,
			"filters":              policy.Filters,
			"levels":               policy.Levels,
			"created_at":           policy.CreatedAt,
			"updated_at":           policy.UpdatedAt,
			"overlapping_warnings": []any{},
		})
	}
}

func deleteEscalationPolicyHandler(svc *Services) gomcp.ToolHandlerFor[deleteEscalationPolicyInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input deleteEscalationPolicyInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		if err := svc.EscalationSvc.DeletePolicy(ctx, input.ID); err != nil {
			return errResult(fmt.Sprintf("policy_not_found: %s", err.Error()))
		}
		return jsonResult(map[string]any{
			"id":      input.ID,
			"deleted": true,
		})
	}
}

func listAlertEscalationRunsHandler(svc *Services) gomcp.ToolHandlerFor[listAlertEscalationRunsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listAlertEscalationRunsInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		runs, err := svc.EscalationSvc.ListRunsForAlert(ctx, input.AlertID)
		if err != nil {
			return nil, nil, fmt.Errorf("list alert escalation runs: %w", err)
		}
		return jsonResult(map[string]any{"runs": runs})
	}
}

func getEscalationRunHandler(svc *Services) gomcp.ToolHandlerFor[getEscalationRunInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getEscalationRunInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		run, err := svc.EscalationSvc.GetRun(ctx, input.ID)
		if err != nil {
			return errResult(fmt.Sprintf("run_not_found: %s", err.Error()))
		}
		return jsonResult(run)
	}
}

func updateEscalationPolicyHandler(svc *Services) gomcp.ToolHandlerFor[updateEscalationPolicyInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input updateEscalationPolicyInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		levels := make([]escalation.LevelReq, len(input.Levels))
		for i, l := range input.Levels {
			levels[i] = escalation.LevelReq{DelaySeconds: l.DelaySeconds, ChannelIDs: l.ChannelIDs}
		}
		scopes := make([]escalation.Scope, len(input.Filters.Scopes))
		for i, s := range input.Filters.Scopes {
			scopes[i] = escalation.Scope{Kind: s.Kind, RefID: s.RefID}
		}
		req := escalation.PolicyRequest{
			Name:   input.Name,
			Active: input.Active,
			Filters: escalation.Filters{
				Severities: input.Filters.Severities,
				Scopes:     scopes,
				Tags:       input.Filters.Tags,
			},
			Levels: levels,
		}
		policy, err := svc.EscalationSvc.UpdatePolicy(ctx, input.ID, req)
		if err != nil {
			return errResult(fmt.Sprintf("update_failed: %s", err.Error()))
		}
		return jsonResult(policy)
	}
}

func setEscalationPolicyActiveHandler(svc *Services) gomcp.ToolHandlerFor[setEscalationPolicyActiveInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input setEscalationPolicyActiveInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkEscalationEdition(); r != nil {
			return r, v, err
		}
		if svc.EscalationSvc == nil {
			return errResult("escalation service not available")
		}
		policy, err := svc.EscalationSvc.SetPolicyActive(ctx, input.ID, input.Active)
		if err != nil {
			return errResult(fmt.Sprintf("set_active_failed: %s", err.Error()))
		}
		return jsonResult(map[string]any{
			"id":         policy.ID,
			"active":     policy.Active,
			"updated_at": policy.UpdatedAt,
		})
	}
}
