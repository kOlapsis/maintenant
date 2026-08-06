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

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSecurityTools(server *gomcp.Server, svc *Services) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_security_insights",
		Description: "List security insights (dangerous runtime configurations) across containers, or for a single container. Includes a severity summary.",
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getSecurityInsightsHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_cve",
		Description: "List active CVE vulnerabilities detected in container images, optionally filtered by container or minimum severity." + requires(extension.CapCVEEnrichment),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listCVEHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "list_risk_scores",
		Description: "List image update risk scores per container (0-100) with their risk level." + requires(extension.CapRiskScoring),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, listRiskScoresHandler(svc))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_security_posture",
		Description: "Get the infrastructure security posture score, or the posture of a single container." + requires(extension.CapSecurityPosture),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, getSecurityPostureHandler(svc))
}

// --- Input types ---

type getSecurityInsightsInput struct {
	ContainerID string `json:"container_id,omitempty" jsonschema:"Internal container ID; omit to return all containers plus a summary"`
}

type listCVEInput struct {
	ContainerID string `json:"container_id,omitempty" jsonschema:"Internal container ID; omit to list across all containers"`
	Severity    string `json:"severity,omitempty" jsonschema:"Minimum severity filter: low, medium, high, or critical"`
}

type listRiskScoresInput struct {
	ContainerID string `json:"container_id,omitempty" jsonschema:"Internal container ID; omit to list all"`
}

type getSecurityPostureInput struct {
	ContainerID string `json:"container_id,omitempty" jsonschema:"Internal container ID; omit for the global infrastructure posture"`
}

// --- Handlers ---

func getSecurityInsightsHandler(svc *Services) gomcp.ToolHandlerFor[getSecurityInsightsInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getSecurityInsightsInput) (*gomcp.CallToolResult, any, error) {
		if svc.SecuritySvc == nil {
			return errResult("security service not available")
		}
		if input.ContainerID != "" {
			// Returns a zero-count object (never nil) when the container has none.
			return jsonResult(svc.SecuritySvc.GetContainerInsights(input.ContainerID))
		}

		total := 0
		if svc.Containers != nil {
			containers, err := svc.Containers.ListContainers(ctx, container.ListContainersOpts{})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list containers: %w", err)
			}
			total = len(containers)
		}
		return jsonResult(map[string]any{
			"insights": svc.SecuritySvc.GetAllInsights(),
			"summary":  svc.SecuritySvc.GetSummary(total),
		})
	}
}

func listCVEHandler(svc *Services) gomcp.ToolHandlerFor[listCVEInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listCVEInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapCVEEnrichment); r != nil {
			return r, v, err
		}
		if svc.UpdateStore == nil {
			return errResult("update store not available")
		}
		if input.ContainerID != "" {
			cves, err := svc.UpdateStore.ListContainerCVEs(ctx, input.ContainerID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list container CVEs: %w", err)
			}
			return jsonResult(cves)
		}
		cves, err := svc.UpdateStore.ListAllActiveCVEs(ctx, update.ListCVEsOpts{Severity: input.Severity})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list CVEs: %w", err)
		}
		return jsonResult(cves)
	}
}

func listRiskScoresHandler(svc *Services) gomcp.ToolHandlerFor[listRiskScoresInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listRiskScoresInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapRiskScoring); r != nil {
			return r, v, err
		}
		if svc.Updates == nil {
			return errResult("update service not available")
		}

		type riskRow struct {
			ContainerID   string `json:"container_id"`
			ContainerName string `json:"container_name"`
			Image         string `json:"image"`
			RiskScore     int    `json:"risk_score"`
			RiskLevel     string `json:"risk_level"`
		}
		toRow := func(u *update.ImageUpdate) riskRow {
			return riskRow{
				ContainerID:   u.ContainerID,
				ContainerName: u.ContainerName,
				Image:         u.Image,
				RiskScore:     u.RiskScore,
				RiskLevel:     string(update.RiskLevelFromScore(u.RiskScore)),
			}
		}

		if input.ContainerID != "" {
			u, err := svc.Updates.GetImageUpdateByContainer(ctx, input.ContainerID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get container risk: %w", err)
			}
			if u == nil {
				return errResult("not found: no risk score for this container")
			}
			return jsonResult(toRow(u))
		}

		updates, err := svc.Updates.ListImageUpdates(ctx, update.ListImageUpdatesOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list risk scores: %w", err)
		}
		rows := make([]riskRow, len(updates))
		for i, u := range updates {
			rows[i] = toRow(u)
		}
		return jsonResult(rows)
	}
}

func getSecurityPostureHandler(svc *Services) gomcp.ToolHandlerFor[getSecurityPostureInput, any] {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getSecurityPostureInput) (*gomcp.CallToolResult, any, error) {
		if r, v, err := checkCapability(extension.CapSecurityPosture); r != nil {
			return r, v, err
		}
		if svc.Scorer == nil || svc.Containers == nil {
			return errResult("security scorer not available")
		}

		if input.ContainerID != "" {
			c, err := svc.Containers.GetContainer(ctx, input.ContainerID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get container: %w", err)
			}
			if c == nil {
				return errResult("not found: container does not exist")
			}
			score, err := svc.Scorer.ScoreContainer(ctx, c.ID, c.ExternalID, c.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to score container: %w", err)
			}
			return jsonResult(score)
		}

		containers, err := svc.Containers.ListContainers(ctx, container.ListContainersOpts{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list containers: %w", err)
		}
		infos := make([]security.ContainerInfo, len(containers))
		for i, c := range containers {
			infos[i] = security.ContainerInfo{ID: c.ID, ExternalID: c.ExternalID, Name: c.Name}
		}
		posture, err := svc.Scorer.ScoreInfrastructure(ctx, infos)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to score infrastructure: %w", err)
		}
		return jsonResult(posture)
	}
}
