// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"time"

	"github.com/kolapsis/maintenant/internal/update"
)

type stubStore struct {
	baseline *update.DigestBaseline
}

func (s *stubStore) InsertScanRecord(_ context.Context, _ *update.ScanRecord) (string, error) {
	return "", nil
}
func (s *stubStore) UpdateScanRecord(_ context.Context, _ *update.ScanRecord) error { return nil }
func (s *stubStore) GetScanRecord(_ context.Context, _ string) (*update.ScanRecord, error) {
	return nil, nil
}
func (s *stubStore) GetLatestScanRecord(_ context.Context) (*update.ScanRecord, error) {
	return nil, nil
}
func (s *stubStore) InsertImageUpdate(_ context.Context, _ *update.ImageUpdate) (string, error) {
	return "", nil
}
func (s *stubStore) UpdateImageUpdate(_ context.Context, _ *update.ImageUpdate) error { return nil }
func (s *stubStore) GetImageUpdate(_ context.Context, _ string) (*update.ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) GetImageUpdateByContainer(_ context.Context, _ string) (*update.ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) ListImageUpdates(_ context.Context, _ update.ListImageUpdatesOpts) ([]*update.ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) GetUpdateSummary(_ context.Context) (*update.UpdateSummary, error) {
	return nil, nil
}
func (s *stubStore) DeleteImageUpdatesByContainer(_ context.Context, _ string) error { return nil }
func (s *stubStore) DeleteStaleImageUpdates(_ context.Context, _ string, _ []string) (int64, error) {
	return 0, nil
}
func (s *stubStore) ListStaleImageUpdates(_ context.Context, _ string, _ []string) ([]update.StaleImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) ListOrphanImageUpdates(_ context.Context) ([]update.StaleImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) DeleteOrphanImageUpdates(_ context.Context) (int64, error) { return 0, nil }
func (s *stubStore) InsertVersionPin(_ context.Context, _ *update.VersionPin) (string, error) {
	return "", nil
}
func (s *stubStore) GetVersionPin(_ context.Context, _ string) (*update.VersionPin, error) {
	return nil, nil
}
func (s *stubStore) DeleteVersionPin(_ context.Context, _ string) error { return nil }
func (s *stubStore) InsertExclusion(_ context.Context, _ *update.UpdateExclusion) (string, error) {
	return "", nil
}
func (s *stubStore) ListExclusions(_ context.Context) ([]*update.UpdateExclusion, error) {
	return nil, nil
}
func (s *stubStore) DeleteExclusion(_ context.Context, _ string) error { return nil }
func (s *stubStore) InsertCVECacheEntry(_ context.Context, _ *update.CVECacheEntry) (string, error) {
	return "", nil
}
func (s *stubStore) GetCVECacheEntries(_ context.Context, _, _, _ string) ([]*update.CVECacheEntry, error) {
	return nil, nil
}
func (s *stubStore) IsCVECacheFresh(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (s *stubStore) UpsertContainerCVE(_ context.Context, _ *update.ContainerCVE) error { return nil }
func (s *stubStore) ListContainerCVEs(_ context.Context, _ string) ([]*update.ContainerCVE, error) {
	return nil, nil
}
func (s *stubStore) ListAllActiveCVEs(_ context.Context, _ update.ListCVEsOpts) ([]*update.ContainerCVE, error) {
	return nil, nil
}
func (s *stubStore) ResolveContainerCVE(_ context.Context, _, _ string) error { return nil }
func (s *stubStore) DeleteContainerCVEs(_ context.Context, _ string) error    { return nil }
func (s *stubStore) GetCVESummaryCounts(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *stubStore) UpsertCVEEvaluation(_ context.Context, _ *update.CVEEvaluation) error { return nil }
func (s *stubStore) GetCVEEvaluation(_ context.Context, _ string) (*update.CVEEvaluation, error) {
	return nil, nil
}
func (s *stubStore) DeleteCVEEvaluation(_ context.Context, _ string) error { return nil }
func (s *stubStore) UpsertDigestBaseline(_ context.Context, _ *update.DigestBaseline) error {
	return nil
}
func (s *stubStore) GetDigestBaseline(_ context.Context, _ string) (*update.DigestBaseline, error) {
	return s.baseline, nil
}
func (s *stubStore) InsertRiskScoreRecord(_ context.Context, _ *update.RiskScoreRecord) (string, error) {
	return "", nil
}
func (s *stubStore) ListRiskScoreHistory(_ context.Context, _ string, _, _ time.Time) ([]*update.RiskScoreRecord, error) {
	return nil, nil
}
func (s *stubStore) CleanupExpired(_ context.Context, _ time.Time) (int64, error) { return 0, nil }
