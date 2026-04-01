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

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// CertificateInfo holds certificate monitor status for scoring.
type CertificateInfo struct {
	Status        string // "valid", "expiring", "expired", "error"
	DaysRemaining int
}

// CVEInfo holds a single CVE for scoring.
type CVEInfo struct {
	CVEID    string
	Severity string // "critical", "high", "medium", "low"
}

// UpdateInfo holds update availability for scoring.
type UpdateInfo struct {
	UpdateType  string // "major", "minor", "patch", "digest_only"
	PublishedAt *time.Time
}

// CertificateReader provides certificate data for a container.
type CertificateReader interface {
	ListCertificatesForContainer(ctx context.Context, containerExternalID string) ([]CertificateInfo, error)
}

// CVEReader provides CVE data for a container.
type CVEReader interface {
	ListCVEsForContainer(ctx context.Context, containerExternalID string) ([]CVEInfo, error)
}

// UpdateReader provides update and image age data for a container.
type UpdateReader interface {
	ListUpdatesForContainer(ctx context.Context, containerExternalID string) ([]UpdateInfo, error)
}

// AcknowledgmentStore persists risk acknowledgments.
type AcknowledgmentStore interface {
	InsertAcknowledgment(ctx context.Context, ack *RiskAcknowledgment) (int64, error)
	DeleteAcknowledgment(ctx context.Context, id int64) error
	ListAcknowledgments(ctx context.Context, containerExternalID string) ([]*RiskAcknowledgment, error)
	GetAcknowledgment(ctx context.Context, id int64) (*RiskAcknowledgment, error)
	IsAcknowledged(ctx context.Context, containerExternalID, findingType, findingKey string) (bool, error)
}

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
	score     *SecurityScore
	expiresAt time.Time
}

// ScorerDeps holds all dependencies for the security Scorer.
type ScorerDeps struct {
	Certs              CertificateReader    // optional — nil skips TLS scoring
	CVEs               CVEReader            // optional — nil skips CVE scoring
	Updates            UpdateReader         // optional — nil skips update scoring
	Security           *Service             // optional — nil skips network exposure scoring
	Acks               AcknowledgmentStore  // required
	Threshold          int                  // optional — 0 disables alerts
	PostureAlertCallback PostureAlertCallback // optional — nil-safe
	PostureEventCallback PostureEventCallback // optional — nil-safe
}

// Scorer computes security posture scores for containers and infrastructure.
type Scorer struct {
	certs   CertificateReader
	cves    CVEReader
	updates UpdateReader
	sec     *Service
	acks    AcknowledgmentStore

	mu             sync.RWMutex
	cache          map[int64]cachedScore
	threshold      int
	lastInfraScore int
	onPostureAlert PostureAlertCallback
	onPostureEvent PostureEventCallback
}

// NewScorer creates a new Scorer with the given data source readers.
// All readers are optional — categories with nil readers are skipped during scoring.
func NewScorer(d ScorerDeps) *Scorer {
	if d.Acks == nil {
		panic("security.NewScorer: Acks is required")
	}
	return &Scorer{
		certs:          d.Certs,
		cves:           d.CVEs,
		updates:        d.Updates,
		sec:            d.Security,
		acks:           d.Acks,
		cache:          make(map[int64]cachedScore),
		threshold:      d.Threshold,
		onPostureAlert: d.PostureAlertCallback,
		onPostureEvent: d.PostureEventCallback,
	}
}

// ScoreContainer computes the security score for a single container.
func (s *Scorer) ScoreContainer(ctx context.Context, containerID int64, containerExternalID string, containerName string) (*SecurityScore, error) {
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

func (s *Scorer) computeContainerScore(ctx context.Context, containerID int64, containerExternalID string, containerName string) (*SecurityScore, error) {
	type categoryResult struct {
		name       string
		weight     int
		subScore   int
		applicable bool
		issueCount int
		summary    string
	}

	categories := make([]categoryResult, 0, 5)
	isPartial := false

	// --- TLS ---
	tlsResult := categoryResult{name: CategoryTLS, weight: WeightTLS}
	if s.certs != nil {
		certs, err := s.certs.ListCertificatesForContainer(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("scoring tls for container %d: %w", containerID, err)
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
		cves, err := s.cves.ListCVEsForContainer(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("scoring cves for container %d: %w", containerID, err)
		}
		if len(cves) > 0 {
			// Filter out acknowledged CVEs
			filtered := filterAcknowledgedCVEs(ctx, cves, containerExternalID, s.acks)
			cveResult.applicable = true
			cveResult.subScore, cveResult.issueCount, cveResult.summary = scoreCVEs(filtered, len(cves)-len(filtered))
		} else {
			// No CVEs means this category applies and scores perfectly
			cveResult.applicable = true
			cveResult.subScore = 100
			cveResult.summary = "no known CVEs"
		}
	}
	categories = append(categories, cveResult)

	// --- Updates ---
	updateResult := categoryResult{name: CategoryUpdates, weight: WeightUpdates}
	imageAgeResult := categoryResult{name: CategoryImageAge, weight: WeightImageAge}
	if s.updates != nil {
		updates, err := s.updates.ListUpdatesForContainer(ctx, containerExternalID)
		if err != nil {
			return nil, fmt.Errorf("scoring updates for container %d: %w", containerID, err)
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

	categoryScores := make([]CategoryScore, len(categories))
	for i, c := range categories {
		categoryScores[i] = CategoryScore{
			Name:       c.name,
			Weight:     c.weight,
			SubScore:   c.subScore,
			Applicable: c.applicable,
			IssueCount: c.issueCount,
			Summary:    c.summary,
		}
		if !c.applicable {
			categoryScores[i].Summary = "not applicable"
		}
	}

	return &SecurityScore{
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

// ContainerInfo holds minimal container data for infrastructure scoring.
type ContainerInfo struct {
	ID         int64
	ExternalID string
	Name       string
}

// ScoreInfrastructure computes the infrastructure-wide security posture.
func (s *Scorer) ScoreInfrastructure(ctx context.Context, containers []ContainerInfo) (*InfrastructurePosture, error) {
	var scores []*SecurityScore
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
		return &InfrastructurePosture{
			Score:          0,
			ColorLevel:     "red",
			ContainerCount: len(containers),
			ScoredCount:    0,
			ComputedAt:     time.Now(),
			Categories:     []CategorySummary{},
			TopRisks:       []ContainerRisk{},
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
	catSummaries := make([]CategorySummary, 0, len(catIssues))
	for name, issues := range catIssues {
		catSummaries = append(catSummaries, CategorySummary{
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
	topRisks := make([]ContainerRisk, topN)
	for i := 0; i < topN; i++ {
		sc := scores[i]
		topRisks[i] = ContainerRisk{
			ContainerID:   sc.ContainerID,
			ContainerName: sc.ContainerName,
			Score:         sc.TotalScore,
			ColorLevel:    sc.ColorLevel,
			TopIssue:      topIssueFromScore(sc),
		}
	}

	return &InfrastructurePosture{
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
func (s *Scorer) InvalidateCache(containerID int64) {
	s.mu.Lock()
	delete(s.cache, containerID)
	s.mu.Unlock()
}

// PostureAlertCallback is called when the infrastructure score crosses a threshold.
type PostureAlertCallback func(score int, previousScore int, color string, isBreach bool)

// PostureEventCallback is called to emit SSE events for posture changes.
type PostureEventCallback func(eventType string, data any)

// SetPostureAlertCallback sets the callback for posture threshold alerts.
func (s *Scorer) SetPostureAlertCallback(cb PostureAlertCallback) {
	s.mu.Lock()
	s.onPostureAlert = cb
	s.mu.Unlock()
}

// SetPostureEventCallback sets the callback for posture SSE events.
func (s *Scorer) SetPostureEventCallback(cb PostureEventCallback) {
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

func scoreTLS(certs []CertificateInfo) (subScore int, issueCount int, summary string) {
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

func scoreCVEs(cves []CVEInfo, acknowledgedCount int) (subScore int, issueCount int, summary string) {
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

func scoreUpdates(updates []UpdateInfo) (subScore int, issueCount int, summary string) {
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

func scoreNetworkExposure(insights []Insight, acknowledgedCount int) (subScore int, issueCount int, summary string) {
	score := 100

	for _, i := range insights {
		switch i.Severity {
		case SeverityCritical:
			score -= 35
		case SeverityHigh:
			score -= 20
		case SeverityMedium:
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

func scoreImageAge(updates []UpdateInfo) (subScore int, issueCount int, summary string) {
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

func filterAcknowledgedCVEs(ctx context.Context, cves []CVEInfo, containerExternalID string, acks AcknowledgmentStore) []CVEInfo {
	if acks == nil {
		return cves
	}
	filtered := make([]CVEInfo, 0, len(cves))
	for _, c := range cves {
		acked, err := acks.IsAcknowledged(ctx, containerExternalID, "cve", c.CVEID)
		if err != nil || !acked {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func filterAcknowledgedInsights(ctx context.Context, insights []Insight, containerExternalID string, acks AcknowledgmentStore) []Insight {
	if acks == nil {
		return insights
	}
	filtered := make([]Insight, 0, len(insights))
	for _, i := range insights {
		key := InsightFindingKey(i)
		acked, err := acks.IsAcknowledged(ctx, containerExternalID, string(i.Type), key)
		if err != nil || !acked {
			filtered = append(filtered, i)
		}
	}
	return filtered
}

// InsightFindingKey returns the dedup key for an insight's finding.
func InsightFindingKey(i Insight) string {
	if port, ok := i.Details["port"]; ok {
		proto := "tcp"
		if p, ok := i.Details["protocol"]; ok {
			proto = fmt.Sprintf("%v", p)
		}
		return fmt.Sprintf("%v/%s", port, proto)
	}
	return ""
}

func topIssueFromScore(sc *SecurityScore) string {
	// Find the worst-scoring applicable category
	worst := CategoryScore{SubScore: 101}
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
