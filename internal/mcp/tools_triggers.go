// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

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
	addTool(server, svc, &gomcp.Tool{
		Name:        "list_triggers",
		Description: "List all alert triggers, oldest first, each with its filters, enabled and notify_on_resolve flags and channel_ids. A trigger sends every alert matching all its non-empty filters to its channels at once; use get_trigger for a single one.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listTriggersHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "get_trigger",
		Description: "Get one alert trigger by id (from list_triggers) with its filters, flags and channel_ids. An unknown id returns \"trigger not found\". Use get_channel to inspect a channel it routes to.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getTriggerHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "create_trigger",
		Description: "Create an alert trigger and return it with its id: every new alert matching all its non-empty filters is sent at once to each enabled channel in channel_ids (from list_channels). It is enabled and relays recoveries by default; the name must be unique and every channel must exist. Use update_trigger to change an existing one, create_escalation_policy for delayed follow-ups." + scopeFiltersRequire(),
		Annotations: &gomcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(false)},
	}, createTriggerHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "update_trigger",
		Description: "Replace an alert trigger by id (from list_triggers): name, the three filters and channel_ids are overwritten, so send them all, and an empty filter matches everything. enabled and notify_on_resolve keep their stored value when omitted. Every channel must exist; concurrent updates are last-write-wins." + scopeFiltersRequire(),
		Annotations: &gomcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)},
	}, updateTriggerHandler(svc))

	addTool(server, svc, &gomcp.Tool{
		Name:        "delete_trigger",
		Description: "Permanently delete an alert trigger by id (from list_triggers); its channels are kept, and alerts stop being routed through it. An unknown id returns \"trigger not found\". To silence it without deleting, use update_trigger with enabled false.",
		Annotations: &gomcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)},
	}, deleteTriggerHandler(svc))
}

// --- input types ---

type listTriggersInput struct{}

type getTriggerInput struct {
	ID string `json:"id" jsonschema:"Trigger ID"`
}

type triggerInput struct {
	Name             string   `json:"name" jsonschema:"Trigger name (1-120 chars)"`
	FilterSeverities string   `json:"filter_severities" jsonschema:"CSV severity filter: info, warning, critical (e.g. 'critical,warning'). Empty matches everything."`
	FilterSources    string   `json:"filter_sources" jsonschema:"CSV source filter (e.g. 'container,endpoint'). Empty matches everything."`
	FilterScopes     string   `json:"filter_scopes" jsonschema:"CSV scope filter of entity_type:entity_id pairs, as found on alerts from list_alerts (e.g. 'container:<entity_id>'). Needs the advanced filters capability. Empty matches everything."`
	Enabled          *bool    `json:"enabled,omitempty" jsonschema:"Whether the trigger is active. Defaults to true on creation; omitted on update, the current value is kept."`
	NotifyOnResolve  *bool    `json:"notify_on_resolve,omitempty" jsonschema:"Relay recovery (resolved) notifications, default true"`
	ChannelIDs       []string `json:"channel_ids" jsonschema:"Notification channel IDs from list_channels (at least one required)"`
}

type updateTriggerInputWithID struct {
	ID string `json:"id" jsonschema:"Trigger ID to update"`
	triggerInput
}

type deleteTriggerInput struct {
	ID string `json:"id" jsonschema:"Trigger ID to delete"`
}

// --- helpers ---

// scopeFiltersRequire tells, in a tool description, which edition scope filters need.
func scopeFiltersRequire() string {
	return " Scope filters require the " + titleEdition(extension.MinEdition(extension.CapAlertAdvancedFilters)) + " edition."
}

// checkAdvancedFilters refuses a scope filter other than the stored one when the
// running edition does not open it. Only the "are advanced filters even in play"
// shortcut lives here; the edition decision itself goes through the registry.
func checkAdvancedFilters(scopes, stored string) (*gomcp.CallToolResult, any, error) {
	if scopes == "" || scopes == stored {
		return nil, nil, nil
	}
	return checkCapability(extension.CapAlertAdvancedFilters)
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
		if r, v, err := checkAdvancedFilters(input.FilterScopes, ""); r != nil {
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

		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		notifyOnResolve := true
		if input.NotifyOnResolve != nil {
			notifyOnResolve = *input.NotifyOnResolve
		}

		t := &alert.AlertTrigger{
			Name:             input.Name,
			FilterSeverities: input.FilterSeverities,
			FilterSources:    input.FilterSources,
			FilterScopes:     input.FilterScopes,
			Enabled:          enabled,
			NotifyOnResolve:  notifyOnResolve,
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
		if r, v, err := checkAdvancedFilters(input.FilterScopes, existing.FilterScopes); r != nil {
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
		if input.Enabled != nil {
			existing.Enabled = *input.Enabled
		}
		if input.NotifyOnResolve != nil {
			existing.NotifyOnResolve = *input.NotifyOnResolve
		}
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
