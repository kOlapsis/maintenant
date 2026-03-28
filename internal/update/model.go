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

package update

import (
	"regexp"
	"time"
)

// UpdateType classifies the type of version update.
type UpdateType string

const (
	UpdateTypeMajor      UpdateType = "major"
	UpdateTypeMinor      UpdateType = "minor"
	UpdateTypePatch      UpdateType = "patch"
	UpdateTypeDigestOnly UpdateType = "digest_only"
	UpdateTypeUnknown    UpdateType = "unknown"
)

// RiskLevel classifies the risk level based on score.
type RiskLevel string

const (
	RiskLevelCritical RiskLevel = "critical"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelModerate RiskLevel = "moderate"
	RiskLevelLow      RiskLevel = "low"
)

// Status represents the lifecycle status of a detected update.
type Status string

const (
	StatusAvailable Status = "available"
	StatusPinned    Status = "pinned"
)

// ScanStatus represents the lifecycle status of a scan cycle.
type ScanStatus string

const (
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
)

// ExclusionType represents the type of exclusion pattern.
type ExclusionType string

const (
	ExclusionTypeImage ExclusionType = "image"
	ExclusionTypeTag   ExclusionType = "tag"
)

// ScanRecord stores the result of each periodic scan cycle.
type ScanRecord struct {
	ID                int64      `json:"id"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ContainersScanned int        `json:"containers_scanned"`
	UpdatesFound      int        `json:"updates_found"`
	Errors            int        `json:"errors"`
	Status            ScanStatus `json:"status"`
}

// ImageUpdate stores a detected update per container image.
type ImageUpdate struct {
	ID                 int64      `json:"id"`
	ScanID             int64      `json:"scan_id"`
	ContainerID        string     `json:"container_id"`
	ContainerName      string     `json:"container_name"`
	Image              string     `json:"image"`
	CurrentTag         string     `json:"current_tag"`
	CurrentDigest      string     `json:"current_digest"`
	Registry           string     `json:"registry"`
	LatestTag          string     `json:"latest_tag,omitempty"`
	LatestDigest       string     `json:"latest_digest,omitempty"`
	UpdateType         UpdateType `json:"update_type,omitempty"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	ChangelogURL       string     `json:"changelog_url,omitempty"`
	ChangelogSummary   string     `json:"changelog_summary,omitempty"`
	HasBreakingChanges bool       `json:"has_breaking_changes"`
	RiskScore          int        `json:"risk_score"`
	PreviousDigest     string     `json:"previous_digest,omitempty"`
	SourceURL          string     `json:"source_url,omitempty"`
	Status             Status     `json:"status"`
	DetectedAt         time.Time  `json:"detected_at"`
}

// BaseRiskScore returns a risk score based on semver update type.
// CE uses this as the final score. Pro can enrich further with CVE data.
func BaseRiskScore(ut UpdateType) int {
	switch ut {
	case UpdateTypeMajor:
		return 85
	case UpdateTypeMinor:
		return 50
	case UpdateTypePatch:
		return 15
	case UpdateTypeDigestOnly:
		return 5
	default:
		return 0
	}
}

// VersionPin tracks a pinned (intentionally frozen) image.
type VersionPin struct {
	ID           int64     `json:"id"`
	ContainerID  string    `json:"container_id"`
	Image        string    `json:"image"`
	PinnedTag    string    `json:"pinned_tag"`
	PinnedDigest string    `json:"pinned_digest"`
	Reason       string    `json:"reason,omitempty"`
	PinnedAt     time.Time `json:"pinned_at"`
}

// UpdateExclusion is a global exclusion rule for images or tags.
type UpdateExclusion struct {
	ID          int64         `json:"id"`
	Pattern     string        `json:"pattern"`
	PatternType ExclusionType `json:"pattern_type"`
	CreatedAt   time.Time     `json:"created_at"`
}

// UpdateResult is the output of scanning a single container.
type UpdateResult struct {
	ContainerID        string
	ContainerName      string
	Image              string
	CurrentTag         string
	CurrentDigest      string
	Registry           string
	LatestTag          string
	LatestDigest       string
	UpdateType         UpdateType
	HasUpdate          bool
	ChangelogURL       string
	ChangelogSummary   string
	HasBreakingChanges bool
	SourceURL          string
	PreviousDigest     string
}

// ScanError represents an error scanning a specific container.
type ScanError struct {
	ContainerID   string
	ContainerName string
	Image         string
	Error         error
}

// CVESeverity classifies the severity of a CVE.
type CVESeverity string

const (
	CVESeverityCritical CVESeverity = "critical"
	CVESeverityHigh     CVESeverity = "high"
	CVESeverityMedium   CVESeverity = "medium"
	CVESeverityLow      CVESeverity = "low"
)

// CVECacheEntry caches CVE lookup results from OSV.dev.
type CVECacheEntry struct {
	ID             int64       `json:"id"`
	Ecosystem      string      `json:"ecosystem"`
	PackageName    string      `json:"package_name"`
	PackageVersion string      `json:"package_version"`
	CVEID          string      `json:"cve_id"`
	CVSSScore      float64     `json:"cvss_score"`
	CVSSVector     string      `json:"cvss_vector"`
	Severity       CVESeverity `json:"severity"`
	Summary        string      `json:"summary"`
	FixedIn        string      `json:"fixed_in"`
	ReferencesJSON string      `json:"references_json"`
	FetchedAt      time.Time   `json:"fetched_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
}

// ContainerCVE links a container to an active CVE.
type ContainerCVE struct {
	ID              int64       `json:"id"`
	ContainerID     string      `json:"container_id"`
	CVEID           string      `json:"cve_id"`
	Severity        CVESeverity `json:"severity"`
	CVSSScore       float64     `json:"cvss_score"`
	Summary         string      `json:"summary"`
	FixedIn         string      `json:"fixed_in"`
	FirstDetectedAt time.Time   `json:"first_detected_at"`
	ResolvedAt      *time.Time  `json:"resolved_at,omitempty"`
}

// ListCVEsOpts contains filter parameters for listing CVEs.
type ListCVEsOpts struct {
	Severity    string
	ContainerID string
}

// RiskScoreRecord stores historical risk scores for trend tracking.
type RiskScoreRecord struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Score       int       `json:"score"`
	FactorsJSON string    `json:"factors_json"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// RiskFactor represents one factor contributing to the risk score.
type RiskFactor struct {
	Label string `json:"label"`
	Score int    `json:"score"`
}

// RiskScore is the computed risk assessment for a container.
type RiskScore struct {
	ContainerID string                `json:"container_id"`
	Score       int                   `json:"score"`
	Level       RiskLevel             `json:"level"`
	Factors     map[string]RiskFactor `json:"factors"`
}

// DigestBaseline stores the last-known remote digest for a non-semver tag.
// Used to detect when a channel tag (e.g. "lts", "alpine") has been republished.
type DigestBaseline struct {
	ContainerID  string    `json:"container_id"`
	Image        string    `json:"image"`
	Tag          string    `json:"tag"`
	RemoteDigest string    `json:"remote_digest"`
	CheckedAt    time.Time `json:"checked_at"`
}

// ReleaseInfo holds information about a GitHub release.
type ReleaseInfo struct {
	TagName            string    `json:"tag_name"`
	Name               string    `json:"name"`
	Body               string    `json:"body"`
	PublishedAt        time.Time `json:"published_at"`
	HTMLURL            string    `json:"html_url"`
	HasBreakingChanges bool      `json:"has_breaking_changes"`
}

// DigestReport is a structured summary of all updates for digest generation.
type DigestReport struct {
	Critical    []ImageUpdate `json:"critical"`
	Recommended []ImageUpdate `json:"recommended"`
	Available   []ImageUpdate `json:"available"`
	UpToDate    int           `json:"up_to_date"`
	Untracked   int           `json:"untracked"`
	TotalCVEs   int           `json:"total_cves"`
}

// RiskLevelFromScore converts a numeric score to a risk level.
func RiskLevelFromScore(score int) RiskLevel {
	switch {
	case score >= 81:
		return RiskLevelCritical
	case score >= 61:
		return RiskLevelHigh
	case score >= 31:
		return RiskLevelModerate
	default:
		return RiskLevelLow
	}
}

// EcosystemResult holds the resolved CVE ecosystem for a container image.
type EcosystemResult struct {
	PackageName     string `json:"package_name"`
	Ecosystem       string `json:"ecosystem"`
	DetectionMethod string `json:"detection_method"`
}

// UpdateConfig holds parsed maintenant.update.* label values.
type UpdateConfig struct {
	Enabled     bool
	Track       string // "major", "minor", "patch", "digest"
	Pin         string // pinned tag
	IgnoreMajor bool
	Registry    string // override registry
	AlertOn     string // "all", "critical", "none"
	DigestOnly  bool
	TagInclude  *regexp.Regexp // compiled tag-include regex, nil if absent/invalid
	TagExclude  *regexp.Regexp // compiled tag-exclude regex, nil if absent/invalid
}
