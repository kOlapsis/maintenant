package mcp

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/extension"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerEditionTools(server *gomcp.Server, svc *Services) {
	addTool(server, svc, &gomcp.Tool{
		Name:        "get_edition",
		Description: "Report the running edition, which capability each edition opens, the quota usage of capped resources, and the resource-history windows. Call this before offering a feature that may be gated, instead of asking the operator which edition they run.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getEditionHandler(svc))
}

type getEditionInput struct{}

func getEditionHandler(svc *Services) gomcp.ToolHandlerFor[getEditionInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, _ getEditionInput) (*gomcp.CallToolResult, any, error) {
		catalog := extension.Catalog()
		features := make(map[string]bool, len(catalog))
		featureEditions := make(map[string]string, len(catalog))
		for c, min := range catalog {
			features[string(c)] = extension.Allows(c)
			featureEditions[string(c)] = string(min)
		}

		maxWindow := extension.MaxHistoryWindow()

		return jsonResult(map[string]any{
			"edition":            string(extension.CurrentEdition()),
			"features":           features,
			"feature_editions":   featureEditions,
			"quotas":             editionQuotas(ctx, svc),
			"tiers":              extension.Tiers(),
			"suspended_channels": editionSuspendedChannels(ctx, svc),
			"resource_history": map[string]any{
				"max_window":         maxWindow.Name,
				"max_window_seconds": int64(maxWindow.Duration / time.Second),
				"windows":            extension.HistoryWindowCatalog(),
			},
		})
	}
}

// editionSuspendedChannels lists the enabled channels the running edition no longer opens.
func editionSuspendedChannels(ctx context.Context, svc *Services) map[string]any {
	list := []alert.SuspendedChannel{}
	if svc.Channels != nil {
		channels, err := svc.Channels.ListChannels(ctx)
		if err != nil {
			svc.logger().Error("failed to list channels for suspension", "error", err)
		} else {
			list = alert.SuspendedChannels(channels)
		}
	}
	return map[string]any{"count": len(list), "channels": list}
}

// editionQuotas reports usage and limit for every capped resource this server
// can count. A limit of -1 means unlimited.
func editionQuotas(ctx context.Context, svc *Services) map[string]any {
	quotas := make(map[string]any, 4)

	count := func(name string, r extension.Resource, used func() (int, error)) {
		n, err := used()
		if err != nil {
			svc.logger().Error("failed to count resource for quota", "resource", string(r), "error", err)
			n = 0
		}
		quotas[name] = map[string]any{"used": n, "limit": extension.Limit(r)}
	}

	if svc.Agents != nil {
		count("agent_hosts", extension.ResourceAgentHosts, func() (int, error) {
			agents, err := svc.Agents.List(ctx, "active")
			return len(agents), err
		})
	}
	if svc.Endpoints != nil {
		count("endpoints", extension.ResourceEndpoints, func() (int, error) {
			return svc.Endpoints.CountStandaloneEndpoints(ctx)
		})
	}
	if svc.Heartbeats != nil {
		count("heartbeats", extension.ResourceHeartbeats, func() (int, error) {
			return svc.Heartbeats.CountActiveHeartbeats(ctx)
		})
	}
	if svc.Certificates != nil {
		count("certificates", extension.ResourceCertificates, func() (int, error) {
			return svc.Certificates.CountStandaloneMonitors(ctx)
		})
	}

	return quotas
}
