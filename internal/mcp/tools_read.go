package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/update"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerReadTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_containers",
		Description: "List all monitored containers with their current state (running, stopped, restarting), health status, and basic metadata.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listContainersHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_container",
		Description: "Get detailed information about a specific container including state, health, image, labels, and recent state transitions.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getContainerHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_container_logs",
		Description: "Get recent log lines from a container's stdout/stderr output.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getContainerLogsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_alerts",
		Description: "List alerts. By default returns only active (unresolved) alerts. Set active_only to false to include recent resolved alerts.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listAlertsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_resources",
		Description: "Get current host resource metrics summary including CPU usage, memory usage, network I/O, and disk usage.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getResourcesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_top_consumers",
		Description: "Get containers ranked by resource consumption (CPU or memory), useful for identifying resource-heavy containers.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getTopConsumersHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_endpoints",
		Description: "List all monitored HTTP/TCP endpoints with their current status (up/down), response time, and uptime percentage.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listEndpointsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_endpoint_history",
		Description: "Get detailed check history for a specific endpoint, including response times, status codes, and error messages.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getEndpointHistoryHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_heartbeats",
		Description: "List all heartbeat/cron monitors with their current status, last ping time, expected period, and grace period.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listHeartbeatsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_certificates",
		Description: "List all monitored TLS certificates with expiration dates, issuer, chain validity, and days until expiry.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listCertificatesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_updates",
		Description: "List available image updates for monitored containers, showing current and latest versions.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getUpdatesHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_health",
		Description: "Check maintenant's own health status, version, and runtime information.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getHealthHandler(svc))
}

// --- Input types ---

type listContainersInput struct{}
type getContainerInput struct {
	ContainerID string `json:"container_id" jsonschema:"Internal container ID"`
}
type getContainerLogsInput struct {
	ContainerID string `json:"container_id" jsonschema:"Internal container ID"`
	Lines       int    `json:"lines,omitempty" jsonschema:"Number of log lines to retrieve, max 1000, default 100"`
	Timestamps  bool   `json:"timestamps,omitempty" jsonschema:"Include timestamps in log output"`
}
type listAlertsInput struct {
	ActiveOnly bool `json:"active_only,omitempty" jsonschema:"Only return active alerts, default true"`
}
type getResourcesInput struct{}
type getTopConsumersInput struct {
	Metric string `json:"metric" jsonschema:"Resource metric to sort by: cpu or memory"`
	Period string `json:"period,omitempty" jsonschema:"Time period for ranking: current, 1h, or 24h"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of containers to return, default 10"`
}
type listEndpointsInput struct{}
type getEndpointHistoryInput struct {
	EndpointID int64 `json:"endpoint_id" jsonschema:"Endpoint ID"`
	Limit      int   `json:"limit,omitempty" jsonschema:"Number of recent checks to return, default 50"`
}
type listHeartbeatsInput struct{}
type listCertificatesInput struct{}
type getUpdatesInput struct{}
type getHealthInput struct{}

// --- Handlers ---

func listContainersHandler(svc *Services) gomcp.ToolHandlerFor[listContainersInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listContainersInput) (*gomcp.CallToolResult, any, error) {
		containers, err := svc.Containers.ListContainers(ctx, container.ListContainersOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list containers: %w", err)
		}
		return jsonResult(containers)
	}
}

func getContainerHandler(svc *Services) gomcp.ToolHandlerFor[getContainerInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getContainerInput) (*gomcp.CallToolResult, any, error) {
		id, err := parseContainerID(input.ContainerID)
		if err != nil {
			return errResult("invalid input: container_id must be a valid integer")
		}
		c, err := svc.Containers.GetContainer(ctx, id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get container: %w", err)
		}
		if c == nil {
			return errResult("not found: container does not exist")
		}
		transitions, _, err := svc.Containers.ListTransitions(ctx, c.ID, container.ListTransitionsOpts{})
		if err != nil {
			svc.Logger.Warn("failed to list transitions for MCP", "container_id", c.ID, "error", err)
			transitions = nil
		}
		result := map[string]any{
			"container":   c,
			"transitions": transitions,
		}
		return jsonResult(result)
	}
}

func getContainerLogsHandler(svc *Services) gomcp.ToolHandlerFor[getContainerLogsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getContainerLogsInput) (*gomcp.CallToolResult, any, error) {
		if svc.LogFetcher == nil {
			return errResult("logs unavailable: no container runtime connected")
		}
		id, err := parseContainerID(input.ContainerID)
		if err != nil {
			return errResult("invalid input: container_id must be a valid integer")
		}
		c, err := svc.Containers.GetContainer(ctx, id)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get container: %w", err)
		}
		if c == nil {
			return errResult("not found: container does not exist")
		}
		lines := input.Lines
		if lines <= 0 {
			lines = 100
		}
		if lines > 1000 {
			lines = 1000
		}
		logLines, err := svc.LogFetcher.FetchLogs(ctx, c.ExternalID, lines, input.Timestamps)
		if err != nil {
			return errResult(fmt.Sprintf("logs unavailable: %s", err.Error()))
		}
		return textResult(strings.Join(logLines, "\n"))
	}
}

func listAlertsHandler(svc *Services) gomcp.ToolHandlerFor[listAlertsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listAlertsInput) (*gomcp.CallToolResult, any, error) {
		// Default to active only
		if input.ActiveOnly || input == (listAlertsInput{}) {
			alerts, err := svc.Alerts.ListActiveAlerts(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list active alerts: %w", err)
			}
			return jsonResult(alerts)
		}
		alerts, err := svc.Alerts.ListAlerts(ctx, alert.ListAlertsOpts{Limit: 100})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list alerts: %w", err)
		}
		return jsonResult(alerts)
	}
}

func getResourcesHandler(svc *Services) gomcp.ToolHandlerFor[getResourcesInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ getResourcesInput) (*gomcp.CallToolResult, any, error) {
		all := svc.Resources.GetAllLatestSnapshots()

		var totalCPU float64
		var totalMemUsed, totalMemLimit int64
		var totalNetRx, totalNetTx int64

		for _, snap := range all {
			totalCPU += snap.CPUPercent
			totalMemUsed += snap.MemUsed
			totalMemLimit += snap.MemLimit
			totalNetRx += snap.NetRxBytes
			totalNetTx += snap.NetTxBytes
		}

		totalMemPercent := 0.0
		if totalMemLimit > 0 {
			totalMemPercent = float64(totalMemUsed) / float64(totalMemLimit) * 100.0
		}

		var diskTotal, diskUsed uint64
		var diskPercent float64
		var fs syscall.Statfs_t
		if err := syscall.Statfs("/", &fs); err == nil {
			diskTotal = fs.Blocks * uint64(fs.Bsize)
			diskFree := fs.Bavail * uint64(fs.Bsize)
			diskUsed = diskTotal - diskFree
			if diskTotal > 0 {
				diskPercent = float64(diskUsed) / float64(diskTotal) * 100.0
			}
		}

		summary := map[string]any{
			"container_count": len(all),
			"cpu_percent":     totalCPU,
			"mem_used":        totalMemUsed,
			"mem_limit":       totalMemLimit,
			"mem_percent":     totalMemPercent,
			"net_rx_bytes":    totalNetRx,
			"net_tx_bytes":    totalNetTx,
			"disk_total":      diskTotal,
			"disk_used":       diskUsed,
			"disk_percent":    diskPercent,
		}
		return jsonResult(summary)
	}
}

func getTopConsumersHandler(svc *Services) gomcp.ToolHandlerFor[getTopConsumersInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getTopConsumersInput) (*gomcp.CallToolResult, any, error) {
		if input.Metric != "cpu" && input.Metric != "memory" {
			return errResult("invalid input: metric must be 'cpu' or 'memory'")
		}
		period := input.Period
		if period == "" {
			period = "current"
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 10
		}
		rows, err := svc.Resources.GetTopConsumersByPeriod(ctx, input.Metric, period, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get top consumers: %w", err)
		}
		return jsonResult(rows)
	}
}

func listEndpointsHandler(svc *Services) gomcp.ToolHandlerFor[listEndpointsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listEndpointsInput) (*gomcp.CallToolResult, any, error) {
		endpoints, err := svc.Endpoints.ListEndpoints(ctx, endpoint.ListEndpointsOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list endpoints: %w", err)
		}
		return jsonResult(endpoints)
	}
}

func getEndpointHistoryHandler(svc *Services) gomcp.ToolHandlerFor[getEndpointHistoryInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getEndpointHistoryInput) (*gomcp.CallToolResult, any, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 50
		}
		checks, _, err := svc.Endpoints.ListCheckResults(ctx, input.EndpointID, endpoint.ListChecksOpts{Limit: limit})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list endpoint checks: %w", err)
		}
		return jsonResult(checks)
	}
}

func listHeartbeatsHandler(svc *Services) gomcp.ToolHandlerFor[listHeartbeatsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listHeartbeatsInput) (*gomcp.CallToolResult, any, error) {
		heartbeats, err := svc.Heartbeats.ListHeartbeats(ctx, heartbeat.ListHeartbeatsOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list heartbeats: %w", err)
		}
		return jsonResult(heartbeats)
	}
}

func listCertificatesHandler(svc *Services) gomcp.ToolHandlerFor[listCertificatesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ listCertificatesInput) (*gomcp.CallToolResult, any, error) {
		certs, err := svc.Certificates.ListMonitors(ctx, certificate.ListCertificatesOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list certificates: %w", err)
		}
		return jsonResult(certs)
	}
}

func getUpdatesHandler(svc *Services) gomcp.ToolHandlerFor[getUpdatesInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ getUpdatesInput) (*gomcp.CallToolResult, any, error) {
		updates, err := svc.Updates.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list updates: %w", err)
		}
		return jsonResult(updates)
	}
}

func getHealthHandler(svc *Services) gomcp.ToolHandlerFor[getHealthInput, any] {
	return func(_ context.Context, _ *gomcp.CallToolRequest, _ getHealthInput) (*gomcp.CallToolResult, any, error) {
		health := map[string]any{
			"status":  "ok",
			"version": svc.Version,
			"runtime": svc.Runtime.Name(),
		}
		return jsonResult(health)
	}
}

// --- Helpers ---

func jsonResult(v any) (*gomcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func textResult(text string) (*gomcp.CallToolResult, any, error) {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}

func errResult(msg string) (*gomcp.CallToolResult, any, error) {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
		IsError: true,
	}, nil, nil
}

func parseContainerID(s string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}
