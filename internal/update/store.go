// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"time"
)

// ErrAmbiguousAgent reports an external id that several remote agents run, so a call has to name the agent.
var ErrAmbiguousAgent = errors.New("several agents run this container")

// UpdateStore defines the persistence interface for update intelligence data.
type UpdateStore interface {
	// Scan records
	InsertScanRecord(ctx context.Context, r *ScanRecord) (string, error)
	UpdateScanRecord(ctx context.Context, r *ScanRecord) error
	GetScanRecord(ctx context.Context, id string) (*ScanRecord, error)
	GetLatestScanRecord(ctx context.Context) (*ScanRecord, error)

	// Image updates
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

	// Version pins
	InsertVersionPin(ctx context.Context, p *VersionPin) (string, error)
	GetVersionPin(ctx context.Context, agentID, containerID string) (*VersionPin, error)
	AdoptVersionPin(ctx context.Context, agentID, containerID, name, image string) (*VersionPin, error)
	DeleteVersionPin(ctx context.Context, agentID, containerID string) error

	// Update exclusions
	CreateExclusion(ctx context.Context, e *UpdateExclusion) (bool, error)
	ListExclusions(ctx context.Context) ([]*UpdateExclusion, error)
	DeleteExclusion(ctx context.Context, id string) error

	// CVE cache
	ReplaceCVECache(ctx context.Context, ecosystem, packageName, packageVersion string, entries []*CVECacheEntry) error
	GetCVECacheEntries(ctx context.Context, ecosystem, packageName, packageVersion string) ([]*CVECacheEntry, error)
	IsCVECacheFresh(ctx context.Context, ecosystem, packageName, packageVersion string) (bool, error)

	// Container CVEs
	UpsertContainerCVE(ctx context.Context, c *ContainerCVE) error
	ResolveContainerCVEs(ctx context.Context, agentID, containerID string, keep []string) error
	ListContainerCVEs(ctx context.Context, agentID, containerID string) ([]*ContainerCVE, error)
	ListAllActiveCVEs(ctx context.Context, opts ListCVEsOpts) ([]*ContainerCVE, error)
	GetCVESummaryCounts(ctx context.Context) (map[string]int, error)

	// CVE evaluations
	UpsertCVEEvaluation(ctx context.Context, e *CVEEvaluation) error
	GetCVEEvaluation(ctx context.Context, agentID, containerID string) (*CVEEvaluation, error)

	// Digest baselines (non-semver tags)
	UpsertDigestBaseline(ctx context.Context, b *DigestBaseline) error
	GetDigestBaseline(ctx context.Context, agentID, containerID string) (*DigestBaseline, error)

	// Risk score history
	InsertRiskScoreRecord(ctx context.Context, r *RiskScoreRecord) (string, error)
	ListRiskScoreHistory(ctx context.Context, agentID, containerID string, from, to time.Time) ([]*RiskScoreRecord, error)

	// ContainerAgent returns the agent a call naming only externalID acts on, or ErrAmbiguousAgent when several remote agents run it.
	ContainerAgent(ctx context.Context, externalID string) (string, error)

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
	Pinned      int `json:"pinned"`
}
