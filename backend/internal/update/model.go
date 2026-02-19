package update

import "time"

// UpdateType classifies the type of version update.
type UpdateType string

const (
	UpdateTypeMajor     UpdateType = "major"
	UpdateTypeMinor     UpdateType = "minor"
	UpdateTypePatch     UpdateType = "patch"
	UpdateTypeDigestOnly UpdateType = "digest_only"
	UpdateTypeUnknown   UpdateType = "unknown"
)

// UpdateStatus represents the lifecycle status of a detected update.
type UpdateStatus string

const (
	UpdateStatusAvailable UpdateStatus = "available"
	UpdateStatusPinned    UpdateStatus = "pinned"
	UpdateStatusDismissed UpdateStatus = "dismissed"
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
	ID                 int64      `json:"id"`
	StartedAt          time.Time  `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	ContainersScanned  int        `json:"containers_scanned"`
	UpdatesFound       int        `json:"updates_found"`
	Errors             int        `json:"errors"`
	Status             ScanStatus `json:"status"`
}

// ImageUpdate stores a detected update per container image.
type ImageUpdate struct {
	ID                 int64        `json:"id"`
	ScanID             int64        `json:"scan_id"`
	ContainerID        string       `json:"container_id"`
	ContainerName      string       `json:"container_name"`
	Image              string       `json:"image"`
	CurrentTag         string       `json:"current_tag"`
	CurrentDigest      string       `json:"current_digest"`
	Registry           string       `json:"registry"`
	LatestTag          string       `json:"latest_tag,omitempty"`
	LatestDigest       string       `json:"latest_digest,omitempty"`
	UpdateType         UpdateType   `json:"update_type,omitempty"`
	RiskScore          int          `json:"risk_score"`
	PublishedAt        *time.Time   `json:"published_at,omitempty"`
	Status             UpdateStatus `json:"status"`
	DetectedAt         time.Time    `json:"detected_at"`
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
	ContainerID   string
	ContainerName string
	Image         string
	CurrentTag    string
	CurrentDigest string
	Registry      string
	LatestTag     string
	LatestDigest  string
	UpdateType    UpdateType
	HasUpdate     bool
}

// ScanError represents an error scanning a specific container.
type ScanError struct {
	ContainerID   string
	ContainerName string
	Image         string
	Error         error
}

// UpdateConfig holds parsed pulseboard.update.* label values.
type UpdateConfig struct {
	Enabled     bool
	Track       string // "major", "minor", "patch", "digest"
	Pin         string // pinned tag
	IgnoreMajor bool
	Registry    string // override registry
	AlertOn     string // "all", "critical", "none"
	DigestOnly  bool
}
