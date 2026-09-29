// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/update"
)

const maxEvaluationErrorLen = 200

// ProEnricher enriches scan results with CVE data, changelog info, and risk scores.
type ProEnricher struct {
	store     update.UpdateStore
	cve       *CVEClient
	changelog *ChangelogResolver
	risk      *RiskEngine
	ecosystem *EcosystemResolver
	logger    *slog.Logger
}

// NewProEnricher creates the full enrichment pipeline.
func NewProEnricher(store update.UpdateStore, cve *CVEClient, changelog *ChangelogResolver, risk *RiskEngine, ecosystem *EcosystemResolver, logger *slog.Logger) *ProEnricher {
	return &ProEnricher{
		store:     store,
		cve:       cve,
		changelog: changelog,
		risk:      risk,
		ecosystem: ecosystem,
		logger:    logger,
	}
}

// Enrich runs a CVE pass for every scanned container, then resolves the
// changelog and risk score of those that actually have an update pending.
func (e *ProEnricher) Enrich(ctx context.Context, results []update.UpdateResult) error {
	for i := range results {
		r := &results[i]
		e.logger.Debug("enriching container",
			"container", r.ContainerName, "image", r.Image,
			"current", r.CurrentTag, "latest", r.LatestTag, "has_update", r.HasUpdate)

		cves := e.enrichCVEs(ctx, r)

		if !r.HasUpdate {
			continue
		}
		e.enrichChangelog(ctx, r)
		e.enrichRisk(ctx, r, cves)
	}

	return nil
}

func (e *ProEnricher) enrichChangelog(ctx context.Context, r *update.UpdateResult) {
	if e.changelog == nil {
		return
	}

	imageRef, _, _ := update.ParseImageRef(r.Image)
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

func (e *ProEnricher) enrichCVEs(ctx context.Context, r *update.UpdateResult) []*update.ContainerCVE {
	if e.cve == nil {
		return nil
	}

	query := e.resolveCVEQuery(ctx, r)
	if query == nil {
		e.logger.Debug("cve: no ecosystem mapping", "container", r.ContainerName, "image", r.Image)
		e.recordEvaluation(ctx, r, &update.CVEEvaluation{Status: update.CVEUnsupported})
		return nil
	}

	cveResults, err := e.cve.QueryCVEs(ctx, []ImageCVEQuery{*query})
	if err != nil {
		e.logger.Warn("cve: query failed", "container", r.ContainerName, "error", err)
		e.recordEvaluation(ctx, r, &update.CVEEvaluation{
			Status:         update.CVEEvaluationError,
			Ecosystem:      query.Ecosystem,
			PackageName:    query.PackageName,
			PackageVersion: query.Version,
			Error:          shortError(err),
		})
		return nil
	}

	e.recordEvaluation(ctx, r, &update.CVEEvaluation{
		Status:         update.CVEEvaluated,
		Ecosystem:      query.Ecosystem,
		PackageName:    query.PackageName,
		PackageVersion: query.Version,
	})

	entries := cveResults[r.ContainerID]
	if len(entries) == 0 {
		e.logger.Debug("cve: no vulnerabilities found", "container", r.ContainerName)
		return nil
	}

	e.logger.Info("cve: vulnerabilities found",
		"container", r.ContainerName, "count", len(entries))

	var cves []*update.ContainerCVE
	now := time.Now()
	for _, entry := range entries {
		cve := &update.ContainerCVE{
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

func (e *ProEnricher) resolveCVEQuery(ctx context.Context, r *update.UpdateResult) *ImageCVEQuery {
	if e.ecosystem == nil {
		return nil
	}
	resolved := e.ecosystem.Resolve(ctx, r.Image, r.CurrentTag, r.CurrentDigest, nil)
	if resolved == nil {
		return nil
	}
	return &ImageCVEQuery{
		ContainerID: r.ContainerID,
		PackageName: resolved.PackageName,
		Ecosystem:   resolved.Ecosystem,
		Version:     r.CurrentTag,
	}
}

func (e *ProEnricher) recordEvaluation(ctx context.Context, r *update.UpdateResult, eval *update.CVEEvaluation) {
	eval.ContainerID = r.ContainerID
	eval.EvaluatedAt = time.Now()
	if err := e.store.UpsertCVEEvaluation(ctx, eval); err != nil {
		e.logger.Warn("cve: failed to persist evaluation", "container", r.ContainerName, "error", err)
	}
}

func shortError(err error) string {
	msg := err.Error()
	if len(msg) > maxEvaluationErrorLen {
		return msg[:maxEvaluationErrorLen]
	}
	return msg
}

func (e *ProEnricher) enrichRisk(ctx context.Context, r *update.UpdateResult, cves []*update.ContainerCVE) {
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
	baseScore := update.BaseRiskScore(u.UpdateType)
	if score.Score < baseScore {
		e.logger.Debug("risk score floored to CE baseline",
			"container", r.ContainerName,
			"pro_score", score.Score, "base_score", baseScore)
		score.Score = baseScore
		score.Level = update.RiskLevelFromScore(baseScore)
		score.Factors["baseline"] = update.RiskFactor{Label: string(u.UpdateType) + "_floor", Score: baseScore}
	}

	e.logger.Debug("risk score calculated",
		"container", r.ContainerName, "score", score.Score, "level", score.Level)

	// Update stored record
	u.RiskScore = score.Score
	if err := e.store.UpdateImageUpdate(ctx, u); err != nil {
		e.logger.Warn("enricher: failed to persist risk score", "container", r.ContainerName, "error", err)
	}

	// Record history
	record := &update.RiskScoreRecord{
		ContainerID: r.ContainerID,
		Score:       score.Score,
		FactorsJSON: FactorsToJSON(score.Factors),
		RecordedAt:  time.Now(),
	}
	if _, err := e.store.InsertRiskScoreRecord(ctx, record); err != nil {
		e.logger.Warn("enricher: failed to persist risk history", "container", r.ContainerName, "error", err)
	}
}
