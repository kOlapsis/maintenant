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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerWriteTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "acknowledge_alert",
		Description: "Acknowledge an active alert so it stops escalating.",
	}, acknowledgeAlertHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_incident",
		Description: "Create a new incident on the status page." + requires(extension.CapIncidents),
	}, createIncidentHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_incident",
		Description: "Post a status update to an existing status page incident." + requires(extension.CapIncidents),
	}, updateIncidentHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_maintenance",
		Description: "Schedule a maintenance window on the status page." + requires(extension.CapMaintenanceWindows),
	}, createMaintenanceHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "pause_monitor",
		Description: "Pause a heartbeat monitor to temporarily stop alerting. Only heartbeat monitors are supported.",
	}, pauseMonitorHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "resume_monitor",
		Description: "Resume a paused heartbeat monitor. Only heartbeat monitors are supported.",
	}, resumeMonitorHandler(svc))
}

// --- Input types ---

type acknowledgeAlertInput struct {
	AlertID        string `json:"alert_id" jsonschema:"Alert ID to acknowledge"`
	AcknowledgedBy string `json:"acknowledged_by,omitempty" jsonschema:"Who is acknowledging the alert (defaults to 'mcp')"`
}

type createIncidentInput struct {
	Title        string   `json:"title" jsonschema:"Incident title"`
	Severity     string   `json:"severity" jsonschema:"Incident severity: minor, major, or critical"`
	Status       string   `json:"status,omitempty" jsonschema:"Initial incident status: investigating, identified, monitoring, or resolved (defaults to investigating)"`
	Message      string   `json:"message,omitempty" jsonschema:"Description or initial update message"`
	ComponentIDs []string `json:"component_ids,omitempty" jsonschema:"IDs of affected status page components"`
}

type updateIncidentInput struct {
	IncidentID string `json:"incident_id" jsonschema:"Incident ID to update"`
	Status     string `json:"status" jsonschema:"New status: investigating, identified, monitoring, or resolved"`
	Message    string `json:"message" jsonschema:"Update message"`
}

type createMaintenanceInput struct {
	Title        string   `json:"title" jsonschema:"Maintenance title"`
	StartTime    string   `json:"start_time" jsonschema:"Start time in RFC 3339 format"`
	EndTime      string   `json:"end_time" jsonschema:"End time in RFC 3339 format"`
	Message      string   `json:"message,omitempty" jsonschema:"Description of maintenance"`
	ComponentIDs []string `json:"component_ids,omitempty" jsonschema:"IDs of affected status page components"`
}

type pauseMonitorInput struct {
	MonitorType string `json:"monitor_type" jsonschema:"Type of monitor, only heartbeat is supported"`
	MonitorID   string `json:"monitor_id" jsonschema:"Monitor ID to pause"`
}

type resumeMonitorInput struct {
	MonitorType string `json:"monitor_type" jsonschema:"Type of monitor, only heartbeat is supported"`
	MonitorID   string `json:"monitor_id" jsonschema:"Monitor ID to resume"`
}

// checkCapability gates a tool behind the capability registry, exactly as the
// REST requireCapability middleware does — same table, same names, so a
// capability resolves identically whichever surface asks. The refusal names the
// edition required; it does not advertise.
// refuseHistoryWindow is the checkCapability of a history window: same shape of
// answer, but the thing refused is a duration, not a flag. This surface has no
// interface in front of it, which is exactly why the cap has to live here too.
func refuseHistoryWindow(w extension.HistoryWindow, required extension.Edition) (*gomcp.CallToolResult, any, error) {
	msg := fmt.Sprintf(
		`{"error":"edition_required","feature":%q,"window":%q,"max_window":%q,"required_edition":%q,"message":"The %s window requires the %s edition of Maintenant."}`,
		string(extension.CapResourceHistory), w.Name, extension.MaxHistoryWindow().Name,
		string(required), w.Name, titleEdition(required),
	)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

func checkCapability(c extension.Capability) (*gomcp.CallToolResult, any, error) {
	if extension.Allows(c) {
		return nil, nil, nil
	}
	required := extension.MinEdition(c)
	msg := fmt.Sprintf(
		`{"error":"edition_required","feature":%q,"required_edition":%q,"message":"This tool requires the %s edition of Maintenant."}`,
		string(c), string(required), titleEdition(required),
	)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

// titleEdition renders an edition for display: "personal" reads as "Personal".
func titleEdition(e extension.Edition) string {
	s := string(e)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- Handlers ---

func acknowledgeAlertHandler(svc *Services) gomcp.ToolHandlerFor[acknowledgeAlertInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input acknowledgeAlertInput) (*gomcp.CallToolResult, any, error) {
		if input.AlertID == "" {
			return errResult("invalid input: alert_id is required")
		}
		if svc.Alerts == nil {
			return errResult("alert store not available")
		}
		a, err := svc.Alerts.GetAlert(ctx, input.AlertID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get alert: %w", err)
		}
		if a == nil {
			return errResult("not found: alert does not exist")
		}
		if a.Status != alert.StatusActive || a.AcknowledgedAt != nil {
			return errResult("conflict: alert is not active or already acknowledged")
		}

		by := input.AcknowledgedBy
		if by == "" {
			by = "mcp"
		}
		now := time.Now().UTC()
		if err := svc.Alerts.AcknowledgeAlert(ctx, input.AlertID, by, now); err != nil {
			return nil, nil, fmt.Errorf("failed to acknowledge alert: %w", err)
		}

		// Best-effort: terminate active escalation runs (Pro). Noop in CE.
		if svc.Escalator != nil {
			if err := svc.Escalator.OnAlertAcknowledged(ctx, input.AlertID, alert.Acknowledgment{By: by, At: now}); err != nil && svc.Logger != nil {
				svc.Logger.Warn("mcp: OnAlertAcknowledged hook error", "error", err, "alert_id", input.AlertID)
			}
		}

		return jsonResult(map[string]any{
			"success":         true,
			"message":         fmt.Sprintf("Alert '%s' acknowledged by %s", input.AlertID, by),
			"acknowledged_at": now.Format(time.RFC3339),
			"acknowledged_by": by,
		})
	}
}

func createIncidentHandler(svc *Services) gomcp.ToolHandlerFor[createIncidentInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createIncidentInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapIncidents); r != nil {
			return r, v, err
		}
		if svc.Incidents == nil {
			return errResult("incident store not available")
		}
		if input.Title == "" || input.Severity == "" {
			return errResult("invalid input: title and severity are required")
		}
		st := input.Status
		if st == "" {
			st = status.IncidentInvestigating
		}

		inc := &status.Incident{Title: input.Title, Severity: input.Severity, Status: st}
		id, err := svc.Incidents.CreateIncident(ctx, inc, input.ComponentIDs, input.Message)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create incident: %w", err)
		}
		if created, _ := svc.Incidents.GetIncident(ctx, id); created != nil {
			inc = created
		}
		return jsonResult(inc)
	}
}

func updateIncidentHandler(svc *Services) gomcp.ToolHandlerFor[updateIncidentInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input updateIncidentInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapIncidents); r != nil {
			return r, v, err
		}
		if svc.Incidents == nil {
			return errResult("incident store not available")
		}
		if input.IncidentID == "" || input.Status == "" || input.Message == "" {
			return errResult("invalid input: incident_id, status and message are required")
		}
		inc, err := svc.Incidents.GetIncident(ctx, input.IncidentID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get incident: %w", err)
		}
		if inc == nil {
			return errResult("not found: incident does not exist")
		}

		upd := &status.IncidentUpdate{IncidentID: input.IncidentID, Status: input.Status, Message: input.Message}
		updateID, err := svc.Incidents.CreateUpdate(ctx, upd)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to post incident update: %w", err)
		}
		upd.ID = updateID
		return jsonResult(upd)
	}
}

func createMaintenanceHandler(svc *Services) gomcp.ToolHandlerFor[createMaintenanceInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createMaintenanceInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapMaintenanceWindows); r != nil {
			return r, v, err
		}
		if svc.Maintenance == nil {
			return errResult("maintenance store not available")
		}
		if input.Title == "" {
			return errResult("invalid input: title is required")
		}
		startsAt, err := time.Parse(time.RFC3339, input.StartTime)
		if err != nil {
			return errResult("invalid input: start_time must be RFC 3339")
		}
		endsAt, err := time.Parse(time.RFC3339, input.EndTime)
		if err != nil {
			return errResult("invalid input: end_time must be RFC 3339")
		}
		if endsAt.Before(startsAt) {
			return errResult("invalid input: end_time must be after start_time")
		}

		mw := &status.MaintenanceWindow{
			Title:       input.Title,
			Description: input.Message,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
		}
		id, err := svc.Maintenance.CreateMaintenance(ctx, mw, input.ComponentIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create maintenance: %w", err)
		}
		if created, _ := svc.Maintenance.GetMaintenance(ctx, id); created != nil {
			mw = created
		}
		return jsonResult(mw)
	}
}

func pauseMonitorHandler(svc *Services) gomcp.ToolHandlerFor[pauseMonitorInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input pauseMonitorInput) (*gomcp.CallToolResult, any, error) {
		if input.MonitorType != "heartbeat" {
			return errResult("invalid input: only 'heartbeat' monitor type is supported")
		}
		hb, err := svc.Heartbeats.PauseHeartbeat(ctx, input.MonitorID)
		if err != nil {
			if errors.Is(err, fmt.Errorf("not found")) {
				return errResult("not found: heartbeat monitor does not exist")
			}
			return nil, nil, fmt.Errorf("failed to pause heartbeat: %w", err)
		}
		if hb == nil {
			return errResult("not found: heartbeat monitor does not exist")
		}
		return jsonResult(map[string]any{"success": true, "message": fmt.Sprintf("Heartbeat '%s' paused", hb.Name)})
	}
}

func resumeMonitorHandler(svc *Services) gomcp.ToolHandlerFor[resumeMonitorInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input resumeMonitorInput) (*gomcp.CallToolResult, any, error) {
		if input.MonitorType != "heartbeat" {
			return errResult("invalid input: only 'heartbeat' monitor type is supported")
		}
		hb, err := svc.Heartbeats.ResumeHeartbeat(ctx, input.MonitorID)
		if err != nil {
			if errors.Is(err, fmt.Errorf("not found")) {
				return errResult("not found: heartbeat monitor does not exist")
			}
			return nil, nil, fmt.Errorf("failed to resume heartbeat: %w", err)
		}
		if hb == nil {
			return errResult("not found: heartbeat monitor does not exist")
		}
		return jsonResult(map[string]any{"success": true, "message": fmt.Sprintf("Heartbeat '%s' resumed", hb.Name)})
	}
}

// requires renders the edition requirement appended to a tool description. It
// reads the registry, so a description cannot claim a tier the gate does not
// enforce — several said "Requires Maintenant Pro" for capabilities that are
// Personal, sending the reader to buy the wrong thing.
func requires(c extension.Capability) string {
	return " Requires the " + titleEdition(extension.MinEdition(c)) + " edition."
}
