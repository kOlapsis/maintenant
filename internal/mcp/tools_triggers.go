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

// Package mcp — alert trigger tools (spec 021)
//
// Tool-to-HTTP mapping:
//
//	list_triggers   → GET    /api/v1/alert-triggers
//	get_trigger     → GET    /api/v1/alert-triggers/{id}
//	create_trigger  → POST   /api/v1/alert-triggers
//	update_trigger  → PUT    /api/v1/alert-triggers/{id}
//	delete_trigger  → DELETE /api/v1/alert-triggers/{id}

package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTriggerTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_triggers",
		Description: "List all alert triggers (filter rules + channel destinations).",
	}, listTriggersHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_trigger",
		Description: "Get a single alert trigger by ID.",
	}, getTriggerHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_trigger",
		Description: "Create a new alert trigger. Filters scopes/tags require Pro edition.",
	}, createTriggerHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_trigger",
		Description: "Update an existing alert trigger (last-write-wins). Filters scopes/tags require Pro edition.",
	}, updateTriggerHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "delete_trigger",
		Description: "Delete an alert trigger. The linked channels are not deleted.",
	}, deleteTriggerHandler(svc))
}

// --- input types ---

type listTriggersInput struct{}

type getTriggerInput struct {
	ID string `json:"id" jsonschema:"Trigger ID"`
}

type triggerInput struct {
	Name             string   `json:"name" jsonschema:"Trigger name (1-120 chars)"`
	FilterSeverities string   `json:"filter_severities" jsonschema:"CSV severity filter (e.g. 'critical,warning'). Empty matches everything."`
	FilterSources    string   `json:"filter_sources" jsonschema:"CSV source filter (e.g. 'container,endpoint'). Empty matches everything."`
	FilterScopes     string   `json:"filter_scopes" jsonschema:"CSV scope filter (e.g. 'container:42,endpoint:7'). Pro only. Empty matches everything."`
	FilterTags       string   `json:"filter_tags" jsonschema:"CSV tag filter. Pro only. Empty matches everything."`
	Enabled          bool     `json:"enabled" jsonschema:"Whether the trigger is active"`
	ChannelIDs       []string `json:"channel_ids" jsonschema:"Notification channel IDs (at least one required)"`
}

type updateTriggerInputWithID struct {
	ID string `json:"id" jsonschema:"Trigger ID to update"`
	triggerInput
}

type deleteTriggerInput struct {
	ID string `json:"id" jsonschema:"Trigger ID to delete"`
}

// --- helpers ---

// checkAdvancedFiltersEdition returns an error result when CE is used with Pro-only filters.
func checkAdvancedFiltersEdition(scopes, tags string) (*gomcp.CallToolResult, any, error) {
	if scopes == "" && tags == "" {
		return nil, nil, nil
	}
	if extension.CurrentEdition() == extension.Enterprise {
		return nil, nil, nil
	}
	msg := `{"error":"edition_required","feature":"alert_advanced_filters","message":"Advanced filters (scopes, tags) require Maintenant Pro. Upgrade at https://maintenant.dev/pro"}`
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

func validateTriggerCommon(t *triggerInput) error {
	if t.Name == "" {
		return errors.New("field=name: required")
	}
	if len(t.Name) > 120 {
		return errors.New("field=name: must be 120 characters or fewer")
	}
	if len(t.ChannelIDs) == 0 {
		return errors.New("field=channel_ids: at least one channel is required")
	}
	return nil
}

// --- handlers ---

func listTriggersHandler(svc *Services) gomcp.ToolHandlerFor[listTriggersInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listTriggersInput) (*gomcp.CallToolResult, any, error) {
		if svc.Triggers == nil {
			return errResult("trigger store not available")
		}
		triggers, err := svc.Triggers.ListTriggers(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list triggers: %w", err)
		}
		if triggers == nil {
			triggers = []*alert.AlertTrigger{}
		}
		return jsonResult(map[string]any{"triggers": triggers})
	}
}

func getTriggerHandler(svc *Services) gomcp.ToolHandlerFor[getTriggerInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getTriggerInput) (*gomcp.CallToolResult, any, error) {
		if svc.Triggers == nil {
			return errResult("trigger store not available")
		}
		t, err := svc.Triggers.GetTrigger(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("get trigger: %w", err)
		}
		if t == nil {
			return errResult("trigger not found")
		}
		return jsonResult(t)
	}
}

func createTriggerHandler(svc *Services) gomcp.ToolHandlerFor[triggerInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input triggerInput) (*gomcp.CallToolResult, any, error) {
		if svc.Triggers == nil || svc.Channels == nil {
			return errResult("trigger or channel store not available")
		}
		if r, v, err := checkAdvancedFiltersEdition(input.FilterScopes, input.FilterTags); r != nil {
			return r, v, err
		}
		if err := validateTriggerCommon(&input); err != nil {
			return errResult(err.Error())
		}
		for _, id := range input.ChannelIDs {
			ch, err := svc.Channels.GetChannel(ctx, id)
			if err != nil {
				return nil, nil, fmt.Errorf("validate channel: %w", err)
			}
			if ch == nil {
				return errResult(fmt.Sprintf("channel %s does not exist", id))
			}
		}

		t := &alert.AlertTrigger{
			Name:             input.Name,
			FilterSeverities: input.FilterSeverities,
			FilterSources:    input.FilterSources,
			FilterScopes:     input.FilterScopes,
			FilterTags:       input.FilterTags,
			Enabled:          input.Enabled,
			ChannelIDs:       input.ChannelIDs,
		}
		if _, err := svc.Triggers.InsertTrigger(ctx, t); err != nil {
			return nil, nil, fmt.Errorf("create trigger: %w", err)
		}
		stored, _ := svc.Triggers.GetTrigger(ctx, t.ID)
		if stored != nil {
			t = stored
		}
		return jsonResult(t)
	}
}

func updateTriggerHandler(svc *Services) gomcp.ToolHandlerFor[updateTriggerInputWithID, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input updateTriggerInputWithID) (*gomcp.CallToolResult, any, error) {
		if svc.Triggers == nil || svc.Channels == nil {
			return errResult("trigger or channel store not available")
		}
		existing, err := svc.Triggers.GetTrigger(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("update trigger: lookup: %w", err)
		}
		if existing == nil {
			return errResult("trigger not found")
		}
		if r, v, err := checkAdvancedFiltersEdition(input.FilterScopes, input.FilterTags); r != nil {
			return r, v, err
		}
		if err := validateTriggerCommon(&input.triggerInput); err != nil {
			return errResult(err.Error())
		}
		for _, id := range input.ChannelIDs {
			ch, err := svc.Channels.GetChannel(ctx, id)
			if err != nil {
				return nil, nil, fmt.Errorf("validate channel: %w", err)
			}
			if ch == nil {
				return errResult(fmt.Sprintf("channel %s does not exist", id))
			}
		}

		existing.Name = input.Name
		existing.FilterSeverities = input.FilterSeverities
		existing.FilterSources = input.FilterSources
		existing.FilterScopes = input.FilterScopes
		existing.FilterTags = input.FilterTags
		existing.Enabled = input.Enabled
		existing.ChannelIDs = input.ChannelIDs

		if err := svc.Triggers.UpdateTrigger(ctx, existing); err != nil {
			return nil, nil, fmt.Errorf("update trigger: %w", err)
		}
		stored, _ := svc.Triggers.GetTrigger(ctx, existing.ID)
		if stored != nil {
			existing = stored
		}
		return jsonResult(existing)
	}
}

func deleteTriggerHandler(svc *Services) gomcp.ToolHandlerFor[deleteTriggerInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input deleteTriggerInput) (*gomcp.CallToolResult, any, error) {
		if svc.Triggers == nil {
			return errResult("trigger store not available")
		}
		existing, err := svc.Triggers.GetTrigger(ctx, input.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("delete trigger: lookup: %w", err)
		}
		if existing == nil {
			return errResult("trigger not found")
		}
		if err := svc.Triggers.DeleteTrigger(ctx, input.ID); err != nil {
			return nil, nil, fmt.Errorf("delete trigger: %w", err)
		}
		return jsonResult(map[string]any{"deleted": true, "id": input.ID})
	}
}
