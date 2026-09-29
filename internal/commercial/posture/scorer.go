// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package posture

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/security"
)

// Category names.
const (
	CategoryTLS             = "tls"
	CategoryCVEs            = "cves"
	CategoryUpdates         = "updates"
	CategoryNetworkExposure = "network_exposure"
	CategoryImageAge        = "image_age"
)

// Category weights (must sum to 100).
const (
	WeightCVEs            = 30
	WeightNetworkExposure = 25
	WeightTLS             = 20
	WeightUpdates         = 15
	WeightImageAge        = 10
)

type cachedScore struct {
	score     *security.SecurityScore
	expiresAt time.Time
}

// ScorerDeps holds all dependencies for the security Scorer.
type ScorerDeps struct {
	Certs                security.CertificateReader    // optional — nil skips TLS scoring
	CVEs                 security.CVEReader            // optional — nil skips CVE scoring
	CVEEvaluations       security.CVEEvaluationReader  // optional — nil makes every CVE category "not evaluated"
	Updates              security.UpdateReader         // optional — nil skips update scoring
	Insights             security.InsightsReader       // optional, nil skips network exposure scoring
	Acks                 security.AcknowledgmentStore  // required
	Threshold            int                           // optional — 0 disables alerts
	PostureAlertCallback security.PostureAlertCallback // optional — nil-safe
	PostureEventCallback security.PostureEventCallback // optional — nil-safe
}

// Scorer computes security posture scores for containers and infrastructure.
type Scorer struct {
	certs   security.CertificateReader
	cves    security.CVEReader
	cveEval security.CVEEvaluationReader
	updates security.UpdateReader
	sec     security.InsightsReader
	acks    security.AcknowledgmentStore

	mu             sync.RWMutex
	cache          map[string]cachedScore
	threshold      int
	lastInfraScore int
	onPostureAlert security.PostureAlertCallback
	onPostureEvent security.PostureEventCallback
}

// NewScorer creates a new Scorer with the given data source readers.
// All readers are optional — categories with nil readers are skipped during scoring.
func NewScorer(d ScorerDeps) *Scorer {
	if d.Acks == nil {
		panic("posture.NewScorer: Acks is required")
	}
	return &Scorer{
		certs:          d.Certs,
		cves:           d.CVEs,
		cveEval:        d.CVEEvaluations,
		updates:        d.Updates,
		sec:            d.Insights,
		acks:           d.Acks,
		cache:          make(map[string]cachedScore),
		threshold:      d.Threshold,
		onPostureAlert: d.PostureAlertCallback,
		onPostureEvent: d.PostureEventCallback,
	}
}

// ScoreContainer computes the security score for a single container.
func (s *Scorer) ScoreContainer(ctx context.Context, containerID string, containerExternalID string, containerName string) (*security.SecurityScore, error) {
	s.mu.RLock()
	if cached, ok := s.cache[containerID]; ok && time.Now().Before(cached.expiresAt) {
		s.mu.RUnlock()
		return cached.score, nil
	}
	s.mu.RUnlock()

	score, err := s.computeContainerScore(ctx, containerID, containerExternalID, containerName)
	if err != nil {
		return nil, err
	}

	if score != nil {
		s.mu.Lock()
		s.cache[containerID] = cachedScore{score: score, expiresAt: time.Now().Add(10 * time.Second)}
		s.mu.Unlock()
	}

	return score, nil
}

func (s *Scorer) computeContainerScore(ctx context.Context, containerID string, containerExternalID string, containerName string) (*security.SecurityScore, error) {
	type categoryResult struct {
		name       string
		weight     int
		subScore   int
		applicable bool
		issueCount int
		summary    string
		evaluation string
	}

	categories := make([]categoryResult, 0, 5)
	isPartial := false

	// --- TLS ---
	tlsResult := categoryResult{name: CategoryTLS, weight: WeightTLS}
	if s.certs != nil {
		certs, err := s.certs.ListCertificatesForContainer(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("scoring tls for container %s: %w", containerID, err)
		}
		if len(certs) > 0 {
			tlsResult.applicable = true
			tlsResult.subScore, tlsResult.issueCount, tlsResult.summary = scoreTLS(certs)
		}
	}
	categories = append(categories, tlsResult)

	// --- CVEs ---
	cveResult := categoryResult{name: CategoryCVEs, weight: WeightCVEs}
	if s.cves != nil {
		state, err := s.cveEvaluationState(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("reading cve evaluation for container %s: %w", containerID, err)
		}
		cveResult.evaluation = state

		switch state {
		case security.EvaluationEvaluated:
			cves, err := s.cves.ListCVEsForContainer(ctx, containerExternalID)
			if err != nil {
				return nil, fmt.Errorf("scoring cves for container %s: %w", containerID, err)
			}
			cveResult.applicable = true
			if len(cves) > 0 {
				filtered := filterAcknowledgedCVEs(ctx, cves, containerExternalID, s.acks)
				cveResult.subScore, cveResult.issueCount, cveResult.summary = scoreCVEs(filtered, len(cves)-len(filtered))
			} else {
				cveResult.subScore = 100
				cveResult.summary = "no known CVEs"
			}
		case security.EvaluationUnsupported:
			cveResult.summary = "image not covered by the vulnerability data source"
		default:
			cveResult.summary = "not evaluated"
			isPartial = true
		}
	}
	categories = append(categories, cveResult)

	// --- Updates ---
	updateResult := categoryResult{name: CategoryUpdates, weight: WeightUpdates}
	imageAgeResult := categoryResult{name: CategoryImageAge, weight: WeightImageAge}
	if s.updates != nil {
		updates, err := s.updates.ListUpdatesForContainer(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("scoring updates for container %s: %w", containerID, err)
		}

		updateResult.applicable = true
		if len(updates) > 0 {
			updateResult.subScore, updateResult.issueCount, updateResult.summary = scoreUpdates(updates)

			// Image age from the most recent update's PublishedAt
			imageAgeResult.applicable = true
			imageAgeResult.subScore, imageAgeResult.issueCount, imageAgeResult.summary = scoreImageAge(updates)
		} else {
			updateResult.subScore = 100
			updateResult.summary = "all images up to date"
			imageAgeResult.applicable = true
			imageAgeResult.subScore = 100
			imageAgeResult.summary = "current image"
		}
	}
	categories = append(categories, updateResult)

	// --- Network Exposure ---
	netResult := categoryResult{name: CategoryNetworkExposure, weight: WeightNetworkExposure}
	if s.sec != nil {
		ci := s.sec.GetContainerInsights(containerID)
		if ci != nil && len(ci.Insights) > 0 {
			// Filter out acknowledged insights
			filtered := filterAcknowledgedInsights(ctx, ci.Insights, containerExternalID, s.acks)
			netResult.applicable = true
			netResult.subScore, netResult.issueCount, netResult.summary = scoreNetworkExposure(filtered, len(ci.Insights)-len(filtered))
		} else {
			netResult.applicable = true
			netResult.subScore = 100
			netResult.summary = "no exposure issues"
		}
	}
	categories = append(categories, netResult)

	// --- Image Age ---
	categories = append(categories, imageAgeResult)

	// Compute weighted score with weight redistribution for non-applicable categories
	totalWeight := 0
	weightedSum := 0
	applicableCount := 0
	for _, c := range categories {
		if c.applicable {
			totalWeight += c.weight
			applicableCount++
		}
	}

	if applicableCount == 0 {
		// No data for any category
		return nil, nil
	}

	for _, c := range categories {
		if c.applicable {
			// Redistribute weight proportionally
			adjustedWeight := float64(c.weight) / float64(totalWeight) * 100
			weightedSum += int(math.Round(float64(c.subScore) * adjustedWeight / 100))
		}
	}

	// Clamp to 0-100
	if weightedSum > 100 {
		weightedSum = 100
	}
	if weightedSum < 0 {
		weightedSum = 0
	}

	categoryScores := make([]security.CategoryScore, len(categories))
	for i, c := range categories {
		categoryScores[i] = security.CategoryScore{
			Name:       c.name,
			Weight:     c.weight,
			SubScore:   c.subScore,
			Applicable: c.applicable,
			IssueCount: c.issueCount,
			Summary:    c.summary,
			Evaluation: c.evaluation,
		}
		if !c.applicable && c.summary == "" {
			categoryScores[i].Summary = "not applicable"
		}
	}

	return &security.SecurityScore{
		ContainerID:     containerID,
		ContainerName:   containerName,
		TotalScore:      weightedSum,
		ColorLevel:      ColorLevel(weightedSum),
		Categories:      categoryScores,
		ApplicableCount: applicableCount,
		ComputedAt:      time.Now(),
		IsPartial:       isPartial,
	}, nil
}

// cveEvaluationState reports how the container's last CVE analysis went; a
// missing reader or a missing row both mean it was never analysed.
func (s *Scorer) cveEvaluationState(ctx context.Context, containerExternalID string) (string, error) {
	if s.cveEval == nil {
		return security.EvaluationNotEvaluated, nil
	}
	eval, err := s.cveEval.GetCVEEvaluation(ctx, containerExternalID)
	if err != nil {
		return "", err
	}
	if eval == nil {
		return security.EvaluationNotEvaluated, nil
	}
	switch eval.Status {
	case security.EvaluationEvaluated, security.EvaluationUnsupported, security.EvaluationError:
		return eval.Status, nil
	default:
		return security.EvaluationNotEvaluated, nil
	}
}

// ScoreInfrastructure computes the infrastructure-wide security posture.
func (s *Scorer) ScoreInfrastructure(ctx context.Context, containers []security.ContainerInfo) (*security.InfrastructurePosture, error) {
	var scores []*security.SecurityScore
	partialCount := 0

	for _, c := range containers {
		score, err := s.ScoreContainer(ctx, c.ID, c.ExternalID, c.Name)
		if err != nil {
			return nil, fmt.Errorf("scoring container %s: %w", c.Name, err)
		}
		if score == nil {
			continue
		}
		scores = append(scores, score)
		if score.IsPartial {
			partialCount++
		}
	}

	if len(scores) == 0 {
		return &security.InfrastructurePosture{
			Score:          0,
			ColorLevel:     "red",
			ContainerCount: len(containers),
			ScoredCount:    0,
			ComputedAt:     time.Now(),
			Categories:     []security.CategorySummary{},
			TopRisks:       []security.ContainerRisk{},
		}, nil
	}

	// Weighted average (equal weight per container)
	totalScore := 0
	for _, sc := range scores {
		totalScore += sc.TotalScore
	}
	avgScore := int(math.Round(float64(totalScore) / float64(len(scores))))

	// Category summaries
	catIssues := make(map[string]int)
	for _, sc := range scores {
		for _, cat := range sc.Categories {
			if cat.Applicable {
				catIssues[cat.Name] += cat.IssueCount
			}
		}
	}
	catSummaries := make([]security.CategorySummary, 0, len(catIssues))
	for name, issues := range catIssues {
		catSummaries = append(catSummaries, security.CategorySummary{
			Name:        name,
			TotalIssues: issues,
			Summary:     fmt.Sprintf("%d issues", issues),
		})
	}
	sort.Slice(catSummaries, func(i, j int) bool {
		return catSummaries[i].TotalIssues > catSummaries[j].TotalIssues
	})

	// Top risks (worst scores first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore < scores[j].TotalScore
	})
	topN := 10
	if len(scores) < topN {
		topN = len(scores)
	}
	topRisks := make([]security.ContainerRisk, topN)
	for i := 0; i < topN; i++ {
		sc := scores[i]
		topRisks[i] = security.ContainerRisk{
			ContainerID:   sc.ContainerID,
			ContainerName: sc.ContainerName,
			Score:         sc.TotalScore,
			ColorLevel:    sc.ColorLevel,
			TopIssue:      topIssueFromScore(sc),
		}
	}

	return &security.InfrastructurePosture{
		Score:          avgScore,
		ColorLevel:     ColorLevel(avgScore),
		ContainerCount: len(containers),
		ScoredCount:    len(scores),
		IsPartial:      partialCount > 0,
		Categories:     catSummaries,
		TopRisks:       topRisks,
		ComputedAt:     time.Now(),
	}, nil
}

// InvalidateCache removes a container's cached score.
func (s *Scorer) InvalidateCache(containerID string) {
	s.mu.Lock()
	delete(s.cache, containerID)
	s.mu.Unlock()
}

// SetPostureAlertCallback sets the callback for posture threshold alerts.
func (s *Scorer) SetPostureAlertCallback(cb security.PostureAlertCallback) {
	s.mu.Lock()
	s.onPostureAlert = cb
	s.mu.Unlock()
}

// SetPostureEventCallback sets the callback for posture SSE events.
func (s *Scorer) SetPostureEventCallback(cb security.PostureEventCallback) {
	s.mu.Lock()
	s.onPostureEvent = cb
	s.mu.Unlock()
}

// Threshold returns the current threshold value.
func (s *Scorer) Threshold() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.threshold
}

// SetThreshold configures the score threshold for alerts. 0 disables alerts.
func (s *Scorer) SetThreshold(threshold int) {
	s.mu.Lock()
	s.threshold = threshold
	s.mu.Unlock()
}

// CheckPostureThreshold compares the current score against the threshold and fires alerts.
func (s *Scorer) CheckPostureThreshold(score int, color string) {
	s.mu.RLock()
	threshold := s.threshold
	previousScore := s.lastInfraScore
	onAlert := s.onPostureAlert
	onEvent := s.onPostureEvent
	s.mu.RUnlock()

	if threshold <= 0 {
		s.mu.Lock()
		s.lastInfraScore = score
		s.mu.Unlock()
		return
	}

	// Emit SSE event on significant change
	delta := score - previousScore
	if delta < 0 {
		delta = -delta
	}
	prevColor := ColorLevel(previousScore)
	if previousScore > 0 && (delta >= 5 || prevColor != color) && onEvent != nil {
		onEvent("security.posture_changed", map[string]any{
			"score":          score,
			"previous_score": previousScore,
			"color":          color,
		})
	}

	// Threshold alert
	if onAlert != nil {
		wasBelowThreshold := previousScore > 0 && previousScore < threshold
		isBelowThreshold := score < threshold

		if isBelowThreshold && !wasBelowThreshold {
			// Score dropped below threshold
			onAlert(score, previousScore, color, true)
		} else if !isBelowThreshold && wasBelowThreshold {
			// Score recovered above threshold
			onAlert(score, previousScore, color, false)
		}
	}

	s.mu.Lock()
	s.lastInfraScore = score
	s.mu.Unlock()
}

// --- Category scoring functions ---

func scoreTLS(certs []security.CertificateInfo) (subScore int, issueCount int, summary string) {
	score := 100
	expiring := 0
	expired := 0
	errCount := 0

	for _, c := range certs {
		switch c.Status {
		case "expired":
			score -= 50
			expired++
		case "expiring":
			if c.DaysRemaining <= 7 {
				score -= 30
			} else if c.DaysRemaining <= 14 {
				score -= 20
			} else {
				score -= 10
			}
			expiring++
		case "error":
			score -= 25
			errCount++
		}
	}

	if score < 0 {
		score = 0
	}

	issues := expired + expiring + errCount
	switch {
	case expired > 0 && expiring > 0:
		summary = fmt.Sprintf("%d expired, %d expiring", expired, expiring)
	case expired > 0:
		summary = fmt.Sprintf("%d expired", expired)
	case expiring > 0:
		summary = fmt.Sprintf("%d expiring within 30 days", expiring)
	case errCount > 0:
		summary = fmt.Sprintf("%d check errors", errCount)
	default:
		summary = "all certificates valid"
	}

	return score, issues, summary
}

func scoreCVEs(cves []security.CVEInfo, acknowledgedCount int) (subScore int, issueCount int, summary string) {
	score := 100
	critical := 0
	high := 0
	medium := 0
	low := 0

	for _, c := range cves {
		switch c.Severity {
		case "critical":
			score -= 30
			critical++
		case "high":
			score -= 15
			high++
		case "medium":
			score -= 5
			medium++
		case "low":
			score -= 2
			low++
		}
	}

	if score < 0 {
		score = 0
	}

	parts := make([]string, 0, 4)
	if critical > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", critical))
	}
	if high > 0 {
		parts = append(parts, fmt.Sprintf("%d high", high))
	}
	if medium > 0 {
		parts = append(parts, fmt.Sprintf("%d medium", medium))
	}
	if low > 0 {
		parts = append(parts, fmt.Sprintf("%d low", low))
	}

	if len(parts) == 0 {
		summary = "no active CVEs"
		if acknowledgedCount > 0 {
			summary = fmt.Sprintf("no active CVEs (%d acknowledged)", acknowledgedCount)
		}
	} else {
		summary = joinParts(parts)
		if acknowledgedCount > 0 {
			summary += fmt.Sprintf(" (%d acknowledged)", acknowledgedCount)
		}
	}

	return score, len(cves), summary
}

func scoreUpdates(updates []security.UpdateInfo) (subScore int, issueCount int, summary string) {
	score := 100
	major := 0
	minor := 0
	patch := 0

	for _, u := range updates {
		switch u.UpdateType {
		case "major":
			score -= 25
			major++
		case "minor":
			score -= 10
			minor++
		case "patch":
			score -= 5
			patch++
		}
	}

	if score < 0 {
		score = 0
	}

	parts := make([]string, 0, 3)
	if major > 0 {
		parts = append(parts, fmt.Sprintf("%d major", major))
	}
	if minor > 0 {
		parts = append(parts, fmt.Sprintf("%d minor", minor))
	}
	if patch > 0 {
		parts = append(parts, fmt.Sprintf("%d patch", patch))
	}

	if len(parts) == 0 {
		summary = "all images up to date"
	} else {
		summary = joinParts(parts) + " available"
	}

	return score, len(updates), summary
}

func scoreNetworkExposure(insights []security.Insight, acknowledgedCount int) (subScore int, issueCount int, summary string) {
	score := 100

	for _, i := range insights {
		switch i.Severity {
		case security.SeverityCritical:
			score -= 35
		case security.SeverityHigh:
			score -= 20
		case security.SeverityMedium:
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}

	if len(insights) == 0 {
		summary = "no exposure issues"
		if acknowledgedCount > 0 {
			summary = fmt.Sprintf("no active issues (%d acknowledged)", acknowledgedCount)
		}
	} else {
		summary = fmt.Sprintf("%d exposure issue(s)", len(insights))
		if acknowledgedCount > 0 {
			summary += fmt.Sprintf(" (%d acknowledged)", acknowledgedCount)
		}
	}

	return score, len(insights), summary
}

func scoreImageAge(updates []security.UpdateInfo) (subScore int, issueCount int, summary string) {
	// Find the most recent PublishedAt from available updates
	var oldest *time.Time
	for _, u := range updates {
		if u.PublishedAt != nil {
			if oldest == nil || u.PublishedAt.Before(*oldest) {
				oldest = u.PublishedAt
			}
		}
	}

	if oldest == nil {
		// No published_at data: neutral score
		return 50, 0, "age unknown"
	}

	daysSincePublished := int(time.Since(*oldest).Hours() / 24)
	if daysSincePublished < 30 {
		return 100, 0, fmt.Sprintf("%d days old", daysSincePublished)
	}

	// Linear decay from 100 at 30 days to 0 at 365 days
	score := 100 - int(math.Round(float64(daysSincePublished-30)/335.0*100))
	if score < 0 {
		score = 0
	}

	issues := 0
	if daysSincePublished > 90 {
		issues = 1
	}

	return score, issues, fmt.Sprintf("%d days old", daysSincePublished)
}

// --- Helper functions ---

func filterAcknowledgedCVEs(ctx context.Context, cves []security.CVEInfo, containerExternalID string, acks security.AcknowledgmentStore) []security.CVEInfo {
	if acks == nil {
		return cves
	}
	filtered := make([]security.CVEInfo, 0, len(cves))
	for _, c := range cves {
		acked, err := acks.IsAcknowledged(ctx, containerExternalID, "cve", c.CVEID)
		if err != nil || !acked {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func filterAcknowledgedInsights(ctx context.Context, insights []security.Insight, containerExternalID string, acks security.AcknowledgmentStore) []security.Insight {
	if acks == nil {
		return insights
	}
	filtered := make([]security.Insight, 0, len(insights))
	for _, i := range insights {
		key := security.InsightFindingKey(i)
		acked, err := acks.IsAcknowledged(ctx, containerExternalID, string(i.Type), key)
		if err != nil || !acked {
			filtered = append(filtered, i)
		}
	}
	return filtered
}

func topIssueFromScore(sc *security.SecurityScore) string {
	// Find the worst-scoring applicable category
	worst := security.CategoryScore{SubScore: 101}
	for _, c := range sc.Categories {
		if c.Applicable && c.SubScore < worst.SubScore {
			worst = c
		}
	}
	if worst.SubScore <= 100 {
		return worst.Summary
	}
	return ""
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += ", " + parts[i]
	}
	return result
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
