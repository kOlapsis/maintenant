// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package security

import "time"

// SecurityScore represents the computed security health score for a single container.
type SecurityScore struct {
	ContainerID     string          `json:"container_id"`
	ContainerName   string          `json:"container_name"`
	TotalScore      int             `json:"score"`
	ColorLevel      string          `json:"color"`
	Categories      []CategoryScore `json:"categories"`
	ApplicableCount int             `json:"applicable_count"`
	ComputedAt      time.Time       `json:"computed_at"`
	IsPartial       bool            `json:"is_partial"`
}

// CategoryScore represents one dimension of the security score.
type CategoryScore struct {
	Name       string `json:"name"`
	Weight     int    `json:"weight"`
	SubScore   int    `json:"sub_score"`
	Applicable bool   `json:"applicable"`
	IssueCount int    `json:"issue_count"`
	Summary    string `json:"summary"`
}

// InfrastructurePosture is the top-level aggregation across all containers.
type InfrastructurePosture struct {
	Score          int               `json:"score"`
	ColorLevel     string            `json:"color"`
	ContainerCount int               `json:"container_count"`
	ScoredCount    int               `json:"scored_count"`
	IsPartial      bool              `json:"is_partial"`
	Categories     []CategorySummary `json:"categories"`
	TopRisks       []ContainerRisk   `json:"top_risks"`
	ComputedAt     time.Time         `json:"computed_at"`
}

// CategorySummary holds aggregated category data across all containers.
type CategorySummary struct {
	Name        string `json:"name"`
	TotalIssues int    `json:"total_issues"`
	Summary     string `json:"summary"`
}

// ContainerRisk is a container's score entry for ranking purposes.
type ContainerRisk struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	Score         int    `json:"score"`
	ColorLevel    string `json:"color"`
	TopIssue      string `json:"top_issue"`
}

// RiskAcknowledgment is a user-created marker indicating a specific risk finding is accepted.
type RiskAcknowledgment struct {
	ID                  string    `json:"id"`
	ContainerExternalID string    `json:"container_external_id"`
	FindingType         string    `json:"finding_type"`
	FindingKey          string    `json:"finding_key"`
	AcknowledgedBy      string    `json:"acknowledged_by"`
	Reason              string    `json:"reason"`
	AcknowledgedAt      time.Time `json:"acknowledged_at"`
}

// ColorLevel returns the color indicator for a given score.
func ColorLevel(score int) string {
	switch {
	case score >= 80:
		return "green"
	case score >= 60:
		return "yellow"
	case score >= 40:
		return "orange"
	default:
		return "red"
	}
}
