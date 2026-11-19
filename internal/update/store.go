package update

import (
	"context"
	"time"
)

type UpdateStore interface {
	InsertScanRecord(ctx context.Context, r *ScanRecord) (string, error)
	UpdateScanRecord(ctx context.Context, r *ScanRecord) error
	GetScanRecord(ctx context.Context, id string) (*ScanRecord, error)
	GetLatestScanRecord(ctx context.Context) (*ScanRecord, error)

	InsertImageUpdate(ctx context.Context, u *ImageUpdate) (string, error)
	UpdateImageUpdate(ctx context.Context, u *ImageUpdate) error
	GetImageUpdate(ctx context.Context, id string) (*ImageUpdate, error)
	GetAgentImageUpdate(ctx context.Context, agentID, containerID string) (*ImageUpdate, error)
	ListImageUpdates(ctx context.Context, opts ListImageUpdatesOpts) ([]*ImageUpdate, error)
	GetUpdateSummary(ctx context.Context) (*UpdateSummary, error)
	DeleteStaleImageUpdates(ctx context.Context, scanID string, agentIDs, scannedContainerNames []string) (int64, error)
	ListStaleImageUpdates(ctx context.Context, scanID string, agentIDs, scannedContainerNames []string) ([]StaleImageUpdate, error)
	ListOrphanImageUpdates(ctx context.Context) ([]StaleImageUpdate, error)
	DeleteOrphanImageUpdates(ctx context.Context) (int64, error)

	InsertVersionPin(ctx context.Context, p *VersionPin) (string, error)
	GetVersionPin(ctx context.Context, agentID, containerID string) (*VersionPin, error)
	DeleteVersionPin(ctx context.Context, agentID, containerID string) error

	CreateExclusion(ctx context.Context, e *UpdateExclusion) (bool, error)
	ListExclusions(ctx context.Context) ([]*UpdateExclusion, error)
	DeleteExclusion(ctx context.Context, id string) error

	InsertCVECacheEntry(ctx context.Context, e *CVECacheEntry) (string, error)
	GetCVECacheEntries(ctx context.Context, ecosystem, packageName, packageVersion string) ([]*CVECacheEntry, error)
	IsCVECacheFresh(ctx context.Context, ecosystem, packageName, packageVersion string) (bool, error)

	UpsertContainerCVE(ctx context.Context, c *ContainerCVE) error
	ListContainerCVEs(ctx context.Context, agentID, containerID string) ([]*ContainerCVE, error)
	ListAllActiveCVEs(ctx context.Context, opts ListCVEsOpts) ([]*ContainerCVE, error)
	GetCVESummaryCounts(ctx context.Context) (map[string]int, error)

	UpsertCVEEvaluation(ctx context.Context, e *CVEEvaluation) error
	GetCVEEvaluation(ctx context.Context, agentID, containerID string) (*CVEEvaluation, error)

	UpsertDigestBaseline(ctx context.Context, b *DigestBaseline) error
	GetDigestBaseline(ctx context.Context, agentID, containerID string) (*DigestBaseline, error)

	InsertRiskScoreRecord(ctx context.Context, r *RiskScoreRecord) (string, error)
	ListRiskScoreHistory(ctx context.Context, agentID, containerID string, from, to time.Time) ([]*RiskScoreRecord, error)

	// ContainerAgent returns the agent of the containers bearing externalID when they all run on one, else the local runtime.
	ContainerAgent(ctx context.Context, externalID string) (string, error)

	CleanupExpired(ctx context.Context, olderThan time.Time) (int64, error)
}

type ListImageUpdatesOpts struct {
	Status     string
	UpdateType string
	MinRisk    int
}

type UpdateSummary struct {
	Critical    int `json:"critical"`
	Recommended int `json:"recommended"`
	Available   int `json:"available"`
	UpToDate    int `json:"up_to_date"`
	Pinned      int `json:"pinned"`
}
