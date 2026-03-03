// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See LICENSE-COMMERCIAL.md
//
// Source: https://github.com/kolapsis/maintenant

package update

import (
	"context"
	"log/slog"
	"time"
)

// ProEnricher enriches scan results with CVE data, changelog info, and risk scores.
type ProEnricher struct {
	store     UpdateStore
	cve       *CVEClient
	changelog *ChangelogResolver
	risk      *RiskEngine
	logger    *slog.Logger
}

// NewProEnricher creates the full enrichment pipeline.
func NewProEnricher(store UpdateStore, cve *CVEClient, changelog *ChangelogResolver, risk *RiskEngine, logger *slog.Logger) *ProEnricher {
	return &ProEnricher{
		store:     store,
		cve:       cve,
		changelog: changelog,
		risk:      risk,
		logger:    logger,
	}
}

// Enrich runs CVE lookup, changelog resolution, and risk scoring for each update result.
func (e *ProEnricher) Enrich(ctx context.Context, results []UpdateResult) error {
	for i := range results {
		if !results[i].HasUpdate {
			continue
		}

		r := &results[i]
		e.logger.Debug("enriching update",
			"container", r.ContainerName, "image", r.Image,
			"current", r.CurrentTag, "latest", r.LatestTag)

		// 1. Changelog resolution
		e.enrichChangelog(ctx, r)

		// 2. CVE lookup
		cves := e.enrichCVEs(ctx, r)

		// 3. Risk scoring
		e.enrichRisk(ctx, r, cves)
	}

	return nil
}

func (e *ProEnricher) enrichChangelog(ctx context.Context, r *UpdateResult) {
	if e.changelog == nil {
		return
	}

	imageRef, _, _ := parseImageRef(r.Image)
	changelogURL, summary, hasBreaking, sourceURL := e.changelog.ResolveChangelog(ctx, imageRef+":"+r.LatestTag, r.LatestTag)

	r.ChangelogURL = changelogURL
	r.ChangelogSummary = summary
	r.HasBreakingChanges = hasBreaking
	r.SourceURL = sourceURL

	if changelogURL != "" {
		e.logger.Debug("changelog resolved",
			"container", r.ContainerName, "url", changelogURL,
			"breaking", hasBreaking)
	}

	// Update the stored record
	u, err := e.store.GetImageUpdateByContainer(ctx, r.ContainerID)
	if err != nil || u == nil {
		return
	}
	u.ChangelogURL = changelogURL
	u.ChangelogSummary = summary
	u.HasBreakingChanges = hasBreaking
	u.SourceURL = sourceURL
	if err := e.store.UpdateImageUpdate(ctx, u); err != nil {
		e.logger.Warn("enricher: failed to persist changelog", "container", r.ContainerName, "error", err)
	}
}

func (e *ProEnricher) enrichCVEs(ctx context.Context, r *UpdateResult) []*ContainerCVE {
	if e.cve == nil {
		return nil
	}

	query := MapImageToCVEQuery(r.Image, r.CurrentTag)
	if query == nil {
		e.logger.Debug("cve: no ecosystem mapping", "container", r.ContainerName, "image", r.Image)
		return nil
	}
	query.ContainerID = r.ContainerID

	cveResults, err := e.cve.QueryCVEs(ctx, []ImageCVEQuery{*query})
	if err != nil {
		e.logger.Warn("cve: query failed", "container", r.ContainerName, "error", err)
		return nil
	}

	entries := cveResults[r.ContainerID]
	if len(entries) == 0 {
		e.logger.Debug("cve: no vulnerabilities found", "container", r.ContainerName)
		return nil
	}

	e.logger.Info("cve: vulnerabilities found",
		"container", r.ContainerName, "count", len(entries))

	// Persist as ContainerCVE records
	var cves []*ContainerCVE
	now := time.Now()
	for _, entry := range entries {
		cve := &ContainerCVE{
			ContainerID:     r.ContainerID,
			CVEID:           entry.CVEID,
			Severity:        entry.Severity,
			CVSSScore:       entry.CVSSScore,
			Summary:         entry.Summary,
			FixedIn:         entry.FixedIn,
			FirstDetectedAt: now,
		}
		if err := e.store.UpsertContainerCVE(ctx, cve); err != nil {
			e.logger.Warn("cve: failed to persist", "cve", entry.CVEID, "error", err)
			continue
		}
		cves = append(cves, cve)
	}

	return cves
}

func (e *ProEnricher) enrichRisk(ctx context.Context, r *UpdateResult, cves []*ContainerCVE) {
	if e.risk == nil {
		return
	}

	u, err := e.store.GetImageUpdateByContainer(ctx, r.ContainerID)
	if err != nil || u == nil {
		return
	}

	riskCtx := RiskContext{
		Criticality: "medium", // default; could be enriched from labels later
	}

	score := e.risk.CalculateScore(u, cves, riskCtx)

	// The Pro score must never downgrade below the CE baseline (semver-based).
	// CE users see BaseRiskScore; Pro enrichment can only raise it with CVE/context data.
	baseScore := BaseRiskScore(u.UpdateType)
	if score.Score < baseScore {
		e.logger.Debug("risk score floored to CE baseline",
			"container", r.ContainerName,
			"pro_score", score.Score, "base_score", baseScore)
		score.Score = baseScore
		score.Level = RiskLevelFromScore(baseScore)
		score.Factors["baseline"] = RiskFactor{Label: string(u.UpdateType) + "_floor", Score: baseScore}
	}

	e.logger.Debug("risk score calculated",
		"container", r.ContainerName, "score", score.Score, "level", score.Level)

	// Update stored record
	u.RiskScore = score.Score
	if err := e.store.UpdateImageUpdate(ctx, u); err != nil {
		e.logger.Warn("enricher: failed to persist risk score", "container", r.ContainerName, "error", err)
	}

	// Record history
	record := &RiskScoreRecord{
		ContainerID: r.ContainerID,
		Score:       score.Score,
		FactorsJSON: FactorsToJSON(score.Factors),
		RecordedAt:  time.Now(),
	}
	if _, err := e.store.InsertRiskScoreRecord(ctx, record); err != nil {
		e.logger.Warn("enricher: failed to persist risk history", "container", r.ContainerName, "error", err)
	}
}
