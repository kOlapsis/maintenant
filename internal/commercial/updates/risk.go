// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"encoding/json"

	"github.com/kolapsis/maintenant/internal/update"
)

// RiskEngine computes contextual risk scores for containers with updates.
type RiskEngine struct{}

// NewRiskEngine creates a risk score engine.
func NewRiskEngine() *RiskEngine {
	return &RiskEngine{}
}

// RiskContext provides monitoring context for risk calculation.
type RiskContext struct {
	HasEndpointCheck bool
	RestartCount     int
	DependentCount   int
	Criticality      string // from maintenant.severity label
}

// CalculateScore computes a risk score (0-100) from update data and monitoring context.
func (re *RiskEngine) CalculateScore(u *update.ImageUpdate, cves []*update.ContainerCVE, rctx RiskContext) update.RiskScore {
	factors := make(map[string]update.RiskFactor)
	total := 0

	// Factor 1: Update type (20% weight, max 20)
	updateScore := 0
	switch u.UpdateType {
	case update.UpdateTypeMajor:
		updateScore = 20
	case update.UpdateTypeMinor:
		updateScore = 12
	case update.UpdateTypePatch:
		updateScore = 6
	case update.UpdateTypeDigestOnly:
		updateScore = 3
	}
	factors["update_type"] = update.RiskFactor{Label: string(u.UpdateType), Score: updateScore}
	total += updateScore

	// Factor 2: CVE severity (30% weight, max 30)
	cveScore := 0
	if len(cves) > 0 {
		maxCVE := 0
		for _, c := range cves {
			switch c.Severity {
			case update.CVESeverityCritical:
				if maxCVE < 30 {
					maxCVE = 30
				}
			case update.CVESeverityHigh:
				if maxCVE < 22 {
					maxCVE = 22
				}
			case update.CVESeverityMedium:
				if maxCVE < 12 {
					maxCVE = 12
				}
			case update.CVESeverityLow:
				if maxCVE < 5 {
					maxCVE = 5
				}
			}
		}
		cveScore = maxCVE
	}
	factors["cve_severity"] = update.RiskFactor{Label: riskSeverityLabel(cves), Score: cveScore}
	total += cveScore

	// Factor 3: Container criticality (15% weight, max 15)
	critScore := 0
	switch rctx.Criticality {
	case "critical":
		critScore = 15
	case "high":
		critScore = 11
	case "medium":
		critScore = 7
	case "low":
		critScore = 3
	default:
		critScore = 7 // default medium
	}
	factors["criticality"] = update.RiskFactor{Label: rctx.Criticality, Score: critScore}
	total += critScore

	// Factor 4: Public exposure (10% weight, max 10)
	exposureScore := 0
	if rctx.HasEndpointCheck {
		exposureScore = 10
	}
	factors["public_exposure"] = update.RiskFactor{Label: riskBoolLabel(rctx.HasEndpointCheck), Score: exposureScore}
	total += exposureScore

	// Factor 5: Stability from restart history (10% weight, max 10)
	stabilityScore := 0
	if rctx.RestartCount > 5 {
		stabilityScore = 10
	} else if rctx.RestartCount > 2 {
		stabilityScore = 6
	} else if rctx.RestartCount > 0 {
		stabilityScore = 3
	}
	factors["stability"] = update.RiskFactor{Label: riskRestartLabel(rctx.RestartCount), Score: stabilityScore}
	total += stabilityScore

	// Factor 6: Dependents (10% weight, max 10)
	depScore := 0
	if rctx.DependentCount > 5 {
		depScore = 10
	} else if rctx.DependentCount > 2 {
		depScore = 6
	} else if rctx.DependentCount > 0 {
		depScore = 3
	}
	factors["dependents"] = update.RiskFactor{Label: riskDepLabel(rctx.DependentCount), Score: depScore}
	total += depScore

	// Factor 7: Breaking changes (5% weight, max 5)
	breakingScore := 0
	if u.HasBreakingChanges {
		breakingScore = 5
	}
	factors["breaking_changes"] = update.RiskFactor{Label: riskBoolLabel(u.HasBreakingChanges), Score: breakingScore}
	total += breakingScore

	if total > 100 {
		total = 100
	}

	return update.RiskScore{
		ContainerID: u.ContainerID,
		Score:       total,
		Level:       update.RiskLevelFromScore(total),
		Factors:     factors,
	}
}

// FactorsToJSON serializes risk factors to JSON for storage.
func FactorsToJSON(factors map[string]update.RiskFactor) string {
	b, _ := json.Marshal(factors)
	return string(b)
}

func riskSeverityLabel(cves []*update.ContainerCVE) string {
	if len(cves) == 0 {
		return "none"
	}
	highest := update.CVESeverityLow
	for _, c := range cves {
		if c.Severity == update.CVESeverityCritical {
			return "critical"
		}
		if c.Severity == update.CVESeverityHigh {
			highest = update.CVESeverityHigh
		} else if c.Severity == update.CVESeverityMedium && highest != update.CVESeverityHigh {
			highest = update.CVESeverityMedium
		}
	}
	return string(highest)
}

func riskBoolLabel(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func riskRestartLabel(count int) string {
	if count == 0 {
		return "stable"
	}
	if count <= 2 {
		return "minor_restarts"
	}
	return "unstable"
}

func riskDepLabel(count int) string {
	if count == 0 {
		return "standalone"
	}
	if count <= 2 {
		return "few"
	}
	return "many"
}
