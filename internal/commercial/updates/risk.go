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

var cveSeverityScores = map[update.CVESeverity]int{
	update.CVESeverityCritical: 90,
	update.CVESeverityHigh:     65,
	update.CVESeverityMedium:   40,
	update.CVESeverityLow:      20,
}

// fixedByUpdate keeps the CVEs whose fix version the update reaches, through its target or its later major.
func fixedByUpdate(r *update.UpdateResult, cves []*update.ContainerCVE) []*update.ContainerCVE {
	var fixed []*update.ContainerCVE
	for _, c := range cves {
		if update.IsFixedByUpdate(r.LatestTag, c.FixedIn) || update.IsFixedByUpdate(r.MajorTag, c.FixedIn) {
			fixed = append(fixed, c)
		}
	}
	return fixed
}

// CalculateScore rates the urgency of an update (0-100): the scan score or the part from the CVEs it fixes, whichever is higher, raised by instability.
func (re *RiskEngine) CalculateScore(r *update.UpdateResult, cves []*update.ContainerCVE, rctx RiskContext) update.RiskScore {
	typeScore := update.BaseRiskScore(r.UpdateType)
	eolScore := 0
	if r.RiskScore > typeScore {
		eolScore = r.RiskScore
	}

	cves = fixedByUpdate(r, cves)
	cveScore := 0
	for _, c := range cves {
		cveScore = max(cveScore, cveSeverityScores[c.Severity])
	}

	exposureScore := 0
	if rctx.PubliclyExposed && len(cves) > 0 {
		exposureScore = 10
	}

	stabilityScore := 0
	if rctx.RestartCount > 5 {
		stabilityScore = 10
	} else if rctx.RestartCount > 2 {
		stabilityScore = 6
	} else if rctx.RestartCount > 0 {
		stabilityScore = 3
	}

	total := min(100, max(r.RiskScore, cveScore+exposureScore)+stabilityScore)
	return update.RiskScore{
		ContainerID: r.ContainerID,
		Score:       total,
		Level:       update.RiskLevelFromScore(total),
		Factors: map[string]update.RiskFactor{
			"update_type":     {Label: string(r.UpdateType), Score: typeScore},
			"end_of_life":     {Label: riskBoolLabel(eolScore > 0), Score: eolScore},
			"cve_severity":    {Label: riskSeverityLabel(cves), Score: cveScore},
			"public_exposure": {Label: riskBoolLabel(rctx.PubliclyExposed), Score: exposureScore},
			"stability":       {Label: riskRestartLabel(rctx.RestartCount), Score: stabilityScore},
		},
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
