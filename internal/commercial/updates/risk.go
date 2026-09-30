// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kolapsis/maintenant/internal/extpoint"
	"github.com/kolapsis/maintenant/internal/security"
	"github.com/kolapsis/maintenant/internal/update"
)

const restartWindow = 24 * time.Hour

// RiskEngine computes contextual risk scores for containers with updates.
type RiskEngine struct {
	insights security.InsightsReader
	restarts extpoint.RestartCounter
}

// NewRiskEngine creates a risk score engine; a nil source leaves its factor at zero.
func NewRiskEngine(insights security.InsightsReader, restarts extpoint.RestartCounter) *RiskEngine {
	return &RiskEngine{insights: insights, restarts: restarts}
}

// RiskContext is what monitoring knows about the container an update targets.
type RiskContext struct {
	PubliclyExposed bool
	RestartCount    int
}

// Context reads the network exposure and the last day's restarts of a container, by its store id.
func (re *RiskEngine) Context(ctx context.Context, containerUID string) (RiskContext, error) {
	var rc RiskContext
	if re.insights != nil {
		rc.PubliclyExposed = networkExposed(re.insights.GetContainerInsights(containerUID))
	}
	if re.restarts != nil {
		n, err := re.restarts.CountRestartsSince(ctx, containerUID, time.Now().Add(-restartWindow))
		if err != nil {
			return rc, err
		}
		rc.RestartCount = n
	}
	return rc, nil
}

func networkExposed(ci *security.ContainerInsights) bool {
	if ci == nil {
		return false
	}
	for _, i := range ci.Insights {
		switch i.Type {
		case security.PortExposedAllInterfaces, security.DatabasePortExposed, security.HostNetworkMode,
			security.ServiceLoadBalancer, security.ServiceNodePort:
			return true
		}
	}
	return false
}

// CalculateScore computes a risk score (0-100) from update data and monitoring context.
func (re *RiskEngine) CalculateScore(u *update.ImageUpdate, cves []*update.ContainerCVE, rctx RiskContext) update.RiskScore {
	factors := make(map[string]update.RiskFactor)
	total := 0

	// Factor 1: Update type (max 20)
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

	// Factor 2: CVE severity (max 30)
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

	// Factor 3: Network exposure (max 10)
	exposureScore := 0
	if rctx.PubliclyExposed {
		exposureScore = 10
	}
	factors["public_exposure"] = update.RiskFactor{Label: riskBoolLabel(rctx.PubliclyExposed), Score: exposureScore}
	total += exposureScore

	// Factor 4: Stability from the restarts of the last day (max 10)
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

	// Factor 5: Breaking changes (max 5)
	breakingScore := 0
	if u.HasBreakingChanges {
		breakingScore = 5
	}
	factors["breaking_changes"] = update.RiskFactor{Label: riskBoolLabel(u.HasBreakingChanges), Score: breakingScore}
	total += breakingScore

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
