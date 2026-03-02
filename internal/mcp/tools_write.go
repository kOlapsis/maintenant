package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kolapsis/maintenant/internal/extension"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerWriteTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "acknowledge_alert",
		Description: "Acknowledge an active alert. Requires extended edition.",
	}, acknowledgeAlertHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_incident",
		Description: "Create a new incident on the status page. Requires extended edition.",
	}, createIncidentHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "update_incident",
		Description: "Update an existing status page incident with a new status and message. Requires extended edition.",
	}, updateIncidentHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "create_maintenance",
		Description: "Schedule a maintenance window on the status page. Requires extended edition.",
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
	AlertID int64 `json:"alert_id" jsonschema:"Alert ID to acknowledge"`
}

type createIncidentInput struct {
	Title   string `json:"title" jsonschema:"Incident title"`
	Status  string `json:"status" jsonschema:"Initial incident status: investigating, identified, monitoring, or resolved"`
	Message string `json:"message" jsonschema:"Description or update message"`
}

type updateIncidentInput struct {
	IncidentID int64  `json:"incident_id" jsonschema:"Incident ID to update"`
	Status     string `json:"status" jsonschema:"New status: investigating, identified, monitoring, or resolved"`
	Message    string `json:"message" jsonschema:"Update message"`
}

type createMaintenanceInput struct {
	Title     string `json:"title" jsonschema:"Maintenance title"`
	StartTime string `json:"start_time" jsonschema:"Start time in RFC 3339 format"`
	EndTime   string `json:"end_time" jsonschema:"End time in RFC 3339 format"`
	Message   string `json:"message,omitempty" jsonschema:"Description of maintenance"`
}

type pauseMonitorInput struct {
	MonitorType string `json:"monitor_type" jsonschema:"Type of monitor, only heartbeat is supported"`
	MonitorID   int64  `json:"monitor_id" jsonschema:"Monitor ID to pause"`
}

type resumeMonitorInput struct {
	MonitorType string `json:"monitor_type" jsonschema:"Type of monitor, only heartbeat is supported"`
	MonitorID   int64  `json:"monitor_id" jsonschema:"Monitor ID to resume"`
}

// --- Handlers ---

func acknowledgeAlertHandler(svc *Services) gomcp.ToolHandlerFor[acknowledgeAlertInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ acknowledgeAlertInput) (*gomcp.CallToolResult, any, error) {
		// Alert acknowledgment is an Enterprise feature — CE has no ack concept
		return errResult(extension.ErrNotAvailable.Error())
	}
}

func createIncidentHandler(svc *Services) gomcp.ToolHandlerFor[createIncidentInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ createIncidentInput) (*gomcp.CallToolResult, any, error) {
		// IncidentManager is noop in CE — the interface has no Create method
		if _, ok := svc.Incidents.(extension.NoopIncidentManager); ok {
			return errResult(extension.ErrNotAvailable.Error())
		}
		// Pro implementations would handle this
		return errResult(extension.ErrNotAvailable.Error())
	}
}

func updateIncidentHandler(svc *Services) gomcp.ToolHandlerFor[updateIncidentInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ updateIncidentInput) (*gomcp.CallToolResult, any, error) {
		if _, ok := svc.Incidents.(extension.NoopIncidentManager); ok {
			return errResult(extension.ErrNotAvailable.Error())
		}
		return errResult(extension.ErrNotAvailable.Error())
	}
}

func createMaintenanceHandler(svc *Services) gomcp.ToolHandlerFor[createMaintenanceInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ createMaintenanceInput) (*gomcp.CallToolResult, any, error) {
		if _, ok := svc.Maintenance.(extension.NoopMaintenanceScheduler); ok {
			return errResult(extension.ErrNotAvailable.Error())
		}
		return errResult(extension.ErrNotAvailable.Error())
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
