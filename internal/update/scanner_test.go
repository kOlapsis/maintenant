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
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ---

// stubRegistry is a fake registryQuerier that returns predefined tag lists and digests.
type stubRegistry struct {
	tags   map[string][]string // imageRef -> tags
	digest string
}

func (r *stubRegistry) ListTags(_ context.Context, imageRef string) ([]string, error) {
	if tags, ok := r.tags[imageRef]; ok {
		return tags, nil
	}
	return []string{}, nil
}

func (r *stubRegistry) GetDigest(_ context.Context, _ string) (string, error) {
	return r.digest, nil
}

// stubStore is a minimal UpdateStore that returns no pins, no exclusions, and no baseline.
type stubStore struct {
	baseline *DigestBaseline
}

func (s *stubStore) InsertScanRecord(_ context.Context, _ *ScanRecord) (int64, error) {
	return 0, nil
}
func (s *stubStore) UpdateScanRecord(_ context.Context, _ *ScanRecord) error { return nil }
func (s *stubStore) GetScanRecord(_ context.Context, _ int64) (*ScanRecord, error) {
	return nil, nil
}
func (s *stubStore) GetLatestScanRecord(_ context.Context) (*ScanRecord, error) { return nil, nil }
func (s *stubStore) InsertImageUpdate(_ context.Context, _ *ImageUpdate) (int64, error) {
	return 0, nil
}
func (s *stubStore) UpdateImageUpdate(_ context.Context, _ *ImageUpdate) error { return nil }
func (s *stubStore) GetImageUpdate(_ context.Context, _ int64) (*ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) GetImageUpdateByContainer(_ context.Context, _ string) (*ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) ListImageUpdates(_ context.Context, _ ListImageUpdatesOpts) ([]*ImageUpdate, error) {
	return nil, nil
}
func (s *stubStore) GetUpdateSummary(_ context.Context) (*UpdateSummary, error) { return nil, nil }
func (s *stubStore) DeleteImageUpdatesByContainer(_ context.Context, _ string) error { return nil }
func (s *stubStore) DeleteStaleImageUpdates(_ context.Context, _ int64, _ []string) (int64, error) {
	return 0, nil
}
func (s *stubStore) InsertVersionPin(_ context.Context, _ *VersionPin) (int64, error) {
	return 0, nil
}
func (s *stubStore) GetVersionPin(_ context.Context, _ string) (*VersionPin, error) { return nil, nil }
func (s *stubStore) DeleteVersionPin(_ context.Context, _ string) error              { return nil }
func (s *stubStore) InsertExclusion(_ context.Context, _ *UpdateExclusion) (int64, error) {
	return 0, nil
}
func (s *stubStore) ListExclusions(_ context.Context) ([]*UpdateExclusion, error) { return nil, nil }
func (s *stubStore) DeleteExclusion(_ context.Context, _ int64) error             { return nil }
func (s *stubStore) InsertCVECacheEntry(_ context.Context, _ *CVECacheEntry) (int64, error) {
	return 0, nil
}
func (s *stubStore) GetCVECacheEntries(_ context.Context, _, _, _ string) ([]*CVECacheEntry, error) {
	return nil, nil
}
func (s *stubStore) IsCVECacheFresh(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (s *stubStore) UpsertContainerCVE(_ context.Context, _ *ContainerCVE) error { return nil }
func (s *stubStore) ListContainerCVEs(_ context.Context, _ string) ([]*ContainerCVE, error) {
	return nil, nil
}
func (s *stubStore) ListAllActiveCVEs(_ context.Context, _ ListCVEsOpts) ([]*ContainerCVE, error) {
	return nil, nil
}
func (s *stubStore) ResolveContainerCVE(_ context.Context, _, _ string) error { return nil }
func (s *stubStore) DeleteContainerCVEs(_ context.Context, _ string) error    { return nil }
func (s *stubStore) GetCVESummaryCounts(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *stubStore) UpsertDigestBaseline(_ context.Context, _ *DigestBaseline) error { return nil }
func (s *stubStore) GetDigestBaseline(_ context.Context, _ string) (*DigestBaseline, error) {
	return s.baseline, nil
}
func (s *stubStore) InsertRiskScoreRecord(_ context.Context, _ *RiskScoreRecord) (int64, error) {
	return 0, nil
}
func (s *stubStore) ListRiskScoreHistory(_ context.Context, _ string, _, _ time.Time) ([]*RiskScoreRecord, error) {
	return nil, nil
}
func (s *stubStore) CleanupExpired(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

// newTestScanner creates a Scanner with a fake registry and stub store.
// delay is set to 0 to avoid throttling in tests.
func newTestScanner(reg registryQuerier, store UpdateStore) *Scanner {
	sc := &Scanner{
		registry: reg,
		store:    store,
		logger:   testLogger(),
		delay:    0,
	}
	return sc
}

// --- T009: tag-include scanner integration test ---

// TestScanner_TagInclude_FiltersTagsBeforeFindBestUpdate verifies that when
// tag-include is set, only matching tags are evaluated by FindBestUpdate.
// A container on nginx:1.24 with include=^1\.25 should find 1.25.1 as an update
// even though 1.26.0 is available — because 1.26.0 doesn't match the pattern.
func TestScanner_TagInclude_FiltersTagsBeforeFindBestUpdate(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.24.1", "1.25.0", "1.25.1", "1.26.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^1\.25`,
			},
		},
	}

	// Manually apply labels to UpdateConfig to simulate label parsing
	containers[0].Labels = map[string]string{
		"maintenant.update.tag-include": `^1\.25`,
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "1.25.1", results[0].LatestTag)
	assert.True(t, results[0].HasUpdate)
}

// TestScanner_TagInclude_NoMatchingTags_NoUpdate verifies that when tag-include
// matches no tags in the registry, no update is reported.
func TestScanner_TagInclude_NoMatchingTags_NoUpdate(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.25.0", "1.26.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^99\.`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	assert.Empty(t, results)
}

// --- T012: exclude-only scanner integration test ---

// TestScanner_TagExclude_VariantFilterPreserved verifies that when only tag-exclude
// is set, the automatic variant filter still applies. A container on nginx:1.24-alpine
// with exclude=(rc|beta) should find 1.26-alpine as update, not 1.26 (no variant).
func TestScanner_TagExclude_VariantFilterPreserved(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {
				"1.24", "1.24-alpine",
				"1.25-rc1", "1.25-alpine",
				"1.26", "1.26-alpine",
			},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24-alpine",
			Labels: map[string]string{
				"maintenant.update.tag-exclude": `rc\d*$`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "1.26-alpine", results[0].LatestTag)
}

// TestScanner_TagExclude_RemovesMatchingTags verifies that tags matching the exclude
// pattern are removed from candidates.
func TestScanner_TagExclude_RemovesMatchingTags(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/redis": {"7.0.0", "7.2.0-rc1", "7.2.0-beta", "7.2.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "redis",
			Image:      "redis:7.0.0",
			Labels: map[string]string{
				"maintenant.update.tag-exclude": `(rc|beta)`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "7.2.0", results[0].LatestTag)
}

// --- T013b: tag filter composition with track ---

// TestScanner_TagFilter_CompositionWithSemver verifies that tag filters reduce the tag
// list first, then FindBestUpdate picks the best semver from the filtered set.
// tag-include=^7\.2 with current=7.0.0 should find the highest 7.2.x tag.
func TestScanner_TagFilter_CompositionWithSemver(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/redis": {"7.0.0", "7.2.0", "7.2.1", "8.0.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "redis",
			Image:      "redis:7.0.0",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^7\.2`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	// 8.0.0 is filtered out by tag-include; best of [7.2.0, 7.2.1] > 7.0.0 → 7.2.1
	assert.Equal(t, "7.2.1", results[0].LatestTag)
}

// --- T016: invalid regex falls back to default behavior ---

// TestScanner_InvalidTagInclude_FallsBackToDefault verifies that an invalid regex
// in tag-include is silently ignored and the scan proceeds with default behavior.
func TestScanner_InvalidTagInclude_FallsBackToDefault(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.25.0", "1.26.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels: map[string]string{
				"maintenant.update.tag-include": `[invalid`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	// Invalid regex → nil include → default behavior (no variant filter either) → 1.26.0 found
	require.Len(t, results, 1)
	assert.Equal(t, "1.26.0", results[0].LatestTag)
}

// TestScanner_InvalidTagExclude_FallsBackToDefault verifies that an invalid regex
// in tag-exclude is silently ignored and the scan proceeds with default behavior.
func TestScanner_InvalidTagExclude_FallsBackToDefault(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.25.0", "1.26.0-rc1", "1.26.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels: map[string]string{
				"maintenant.update.tag-exclude": `[bad`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	// Invalid regex → nil exclude → no filter → 1.26.0 found (semver sorts pre-releases out)
	require.Len(t, results, 1)
	assert.Equal(t, "1.26.0", results[0].LatestTag)
}

// --- T017: digest-only mode bypass ---

// TestScanner_DigestOnlyMode_TagFilterBypassed verifies that tag filter labels are
// ignored when the container uses a non-semver channel tag like "latest".
// The digest comparison should proceed normally even if tag-include is set.
// Uses docker.io/library/nginx:latest to avoid the local-image heuristic check
// (which skips single-component images tagged "latest").
func TestScanner_DigestOnlyMode_TagFilterBypassed(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"latest", "1.25.0", "1.26.0"},
		},
		digest: "sha256:aabbccdd11223344",
	}

	// Provide an old baseline so digest change is detected
	oldBaseline := &DigestBaseline{
		ContainerID:  "ctr1",
		Image:        "docker.io/library/nginx:latest",
		Tag:          "latest",
		RemoteDigest: "sha256:00112233445566778",
		CheckedAt:    time.Now().Add(-24 * time.Hour),
	}
	sc := newTestScanner(reg, &stubStore{baseline: oldBaseline})

	// tag-include set to semver pattern — should NOT affect "latest" digest comparison.
	// Use docker.io/library/nginx:latest so imageRef is "library/nginx" (contains "/"),
	// bypassing the local-image heuristic that skips bare "nginx:latest".
	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "docker.io/library/nginx:latest",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^1\.25`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	// Digest changed → update detected despite tag-include being set
	require.Len(t, results, 1)
	assert.Equal(t, UpdateTypeDigestOnly, results[0].UpdateType)
	assert.Equal(t, "latest", results[0].LatestTag)
	assert.True(t, results[0].HasUpdate)
}

// TestScanner_DigestOnlyMode_NoBaselineChange_NoUpdate verifies that when the digest
// hasn't changed, no update is reported regardless of tag-include being set.
func TestScanner_DigestOnlyMode_NoBaselineChange_NoUpdate(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"latest", "1.25.0"},
		},
		digest: "sha256:aabbccdd11223344",
	}

	existingBaseline := &DigestBaseline{
		ContainerID:  "ctr1",
		Image:        "docker.io/library/nginx:latest",
		Tag:          "latest",
		RemoteDigest: "sha256:aabbccdd11223344",
		CheckedAt:    time.Now().Add(-1 * time.Hour),
	}
	sc := newTestScanner(reg, &stubStore{baseline: existingBaseline})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "docker.io/library/nginx:latest",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^1\.2`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	assert.Empty(t, results)
}

// --- Combined include+exclude ---

// TestScanner_IncludeAndExclude_BothApply verifies that when both labels are set,
// include filters first, then exclude removes from the included set.
func TestScanner_IncludeAndExclude_BothApply(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/node": {
				"20.0.0", "20.1.0", "20.2.0-rc1", "20.3.0",
				"21.0.0", "22.0.0",
			},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "node",
			Image:      "node:20.0.0",
			Labels: map[string]string{
				"maintenant.update.tag-include": `^20\.`,
				"maintenant.update.tag-exclude": `rc`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	// Include keeps [20.0.0, 20.1.0, 20.2.0-rc1, 20.3.0]
	// Exclude removes [20.2.0-rc1]
	// FindBestUpdate picks 20.3.0 (semver skips prereleases anyway, but exclude removes rc first)
	assert.Equal(t, "20.3.0", results[0].LatestTag)
}

// --- No labels — default behavior preserved ---

// TestScanner_NoLabels_DefaultBehavior verifies that without tag filter labels,
// the scanner uses the existing default behavior (variant filter via FindBestUpdate).
func TestScanner_NoLabels_DefaultBehavior(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.25.0", "1.26.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels:     nil,
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "1.26.0", results[0].LatestTag)
}

// --- Disabled tracking ---

// TestScanner_UpdateDisabled_Skipped verifies that a container with
// maintenant.update.enabled=false is skipped even with tag-include set.
func TestScanner_UpdateDisabled_Skipped(t *testing.T) {
	reg := &stubRegistry{
		tags: map[string][]string{
			"library/nginx": {"1.24.0", "1.25.0"},
		},
	}
	sc := newTestScanner(reg, &stubStore{})

	containers := []ContainerInfo{
		{
			ExternalID: "ctr1",
			Name:       "nginx",
			Image:      "nginx:1.24.0",
			Labels: map[string]string{
				"maintenant.update.enabled":     "false",
				"maintenant.update.tag-include": `^1\.25`,
			},
		},
	}

	results, errs := sc.Scan(context.Background(), containers)
	require.Empty(t, errs)
	assert.Empty(t, results)
}

// Ensure *regexp.Regexp is assignable to cfg fields (compile-time check for T015 coverage).
var _ = func() {
	re := regexp.MustCompile(`test`)
	cfg := UpdateConfig{TagInclude: re, TagExclude: re}
	_ = cfg
}
