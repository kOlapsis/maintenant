package update

import (
	"context"
	"time"
)

// UpdateStore defines the persistence interface for update intelligence data.
type UpdateStore interface {
	// Scan records
	InsertScanRecord(ctx context.Context, r *ScanRecord) (int64, error)
	UpdateScanRecord(ctx context.Context, r *ScanRecord) error
	GetScanRecord(ctx context.Context, id int64) (*ScanRecord, error)
	GetLatestScanRecord(ctx context.Context) (*ScanRecord, error)

	// Image updates
	InsertImageUpdate(ctx context.Context, u *ImageUpdate) (int64, error)
	UpdateImageUpdate(ctx context.Context, u *ImageUpdate) error
	GetImageUpdate(ctx context.Context, id int64) (*ImageUpdate, error)
	GetImageUpdateByContainer(ctx context.Context, containerID string) (*ImageUpdate, error)
	ListImageUpdates(ctx context.Context, opts ListImageUpdatesOpts) ([]*ImageUpdate, error)
	GetUpdateSummary(ctx context.Context) (*UpdateSummary, error)
	DeleteImageUpdatesByContainer(ctx context.Context, containerID string) error

	// Version pins
	InsertVersionPin(ctx context.Context, p *VersionPin) (int64, error)
	GetVersionPin(ctx context.Context, containerID string) (*VersionPin, error)
	DeleteVersionPin(ctx context.Context, containerID string) error

	// Update exclusions
	InsertExclusion(ctx context.Context, e *UpdateExclusion) (int64, error)
	ListExclusions(ctx context.Context) ([]*UpdateExclusion, error)
	DeleteExclusion(ctx context.Context, id int64) error

	// Retention cleanup
	CleanupExpired(ctx context.Context, olderThan time.Time) (int64, error)
}

// ListImageUpdatesOpts contains filter parameters for listing image updates.
type ListImageUpdatesOpts struct {
	Status     string
	UpdateType string
}

// UpdateSummary holds aggregated update counts.
type UpdateSummary struct {
	Critical    int `json:"critical"`
	Recommended int `json:"recommended"`
	Available   int `json:"available"`
	UpToDate    int `json:"up_to_date"`
	Untracked   int `json:"untracked"`
	Pinned      int `json:"pinned"`
}
