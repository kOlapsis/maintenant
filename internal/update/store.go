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
	DeleteStaleImageUpdates(ctx context.Context, scanID int64, scannedContainerNames []string) (int64, error)

	// Version pins
	InsertVersionPin(ctx context.Context, p *VersionPin) (int64, error)
	GetVersionPin(ctx context.Context, containerID string) (*VersionPin, error)
	DeleteVersionPin(ctx context.Context, containerID string) error

	// Update exclusions
	InsertExclusion(ctx context.Context, e *UpdateExclusion) (int64, error)
	ListExclusions(ctx context.Context) ([]*UpdateExclusion, error)
	DeleteExclusion(ctx context.Context, id int64) error

	// CVE cache
	InsertCVECacheEntry(ctx context.Context, e *CVECacheEntry) (int64, error)
	GetCVECacheEntries(ctx context.Context, ecosystem, packageName, packageVersion string) ([]*CVECacheEntry, error)
	IsCVECacheFresh(ctx context.Context, ecosystem, packageName, packageVersion string) (bool, error)

	// Container CVEs
	UpsertContainerCVE(ctx context.Context, c *ContainerCVE) error
	ListContainerCVEs(ctx context.Context, containerID string) ([]*ContainerCVE, error)
	ListAllActiveCVEs(ctx context.Context, opts ListCVEsOpts) ([]*ContainerCVE, error)
	ResolveContainerCVE(ctx context.Context, containerID, cveID string) error
	DeleteContainerCVEs(ctx context.Context, containerID string) error
	GetCVESummaryCounts(ctx context.Context) (map[string]int, error)

	// Digest baselines (non-semver tags)
	UpsertDigestBaseline(ctx context.Context, b *DigestBaseline) error
	GetDigestBaseline(ctx context.Context, containerID string) (*DigestBaseline, error)

	// Risk score history
	InsertRiskScoreRecord(ctx context.Context, r *RiskScoreRecord) (int64, error)
	ListRiskScoreHistory(ctx context.Context, containerID string, from, to time.Time) ([]*RiskScoreRecord, error)

	// Retention cleanup
	CleanupExpired(ctx context.Context, olderThan time.Time) (int64, error)
}

// ListImageUpdatesOpts contains filter parameters for listing image updates.
type ListImageUpdatesOpts struct {
	Status     string
	UpdateType string
	MinRisk    int
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
