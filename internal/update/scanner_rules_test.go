// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oldBaseline(containerID, tag string) *DigestBaseline {
	return &DigestBaseline{ContainerID: containerID, Tag: tag, RemoteDigest: "sha256:old"}
}

func TestScanner_OfficialImageOnLatest_IsChecked(t *testing.T) {
	for _, image := range []string{"nginx", "nginx:latest", "redis:latest"} {
		t.Run(image, func(t *testing.T) {
			reg := &stubRegistry{
				tags: map[string][]string{
					"library/nginx": {"latest", "1.27.0"},
					"library/redis": {"latest", "7.4.0"},
				},
				digest: "sha256:new",
			}
			sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "latest")})

			results, errs := sc.Scan(context.Background(), []ContainerInfo{
				{ExternalID: "ctr1", Name: "web", Image: image},
			})
			require.Empty(t, errs)
			require.Len(t, results, 1)
			assert.Equal(t, UpdateTypeDigestOnly, results[0].UpdateType)
			assert.Equal(t, "latest", results[0].LatestTag)
		})
	}
}

func TestScanner_TrackLabels_LimitTheVersionJump(t *testing.T) {
	cases := []struct {
		name     string
		labels   map[string]string
		wantTag  string
		wantType UpdateType
	}{
		{"no label", nil, "2.0.0", UpdateTypeMajor},
		{"track major", map[string]string{"maintenant.update.track": "major"}, "2.0.0", UpdateTypeMajor},
		{"track minor", map[string]string{"maintenant.update.track": "minor"}, "1.3.1", UpdateTypeMinor},
		{"track patch", map[string]string{"maintenant.update.track": "patch"}, "1.2.5", UpdateTypePatch},
		{"ignore_major", map[string]string{"maintenant.update.ignore_major": "true"}, "1.3.1", UpdateTypeMinor},
		{"ignore_major with track patch", map[string]string{
			"maintenant.update.ignore_major": "true",
			"maintenant.update.track":        "patch",
		}, "1.2.5", UpdateTypePatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &stubRegistry{tags: map[string][]string{
				"library/postgres": {"1.2.3", "1.2.4", "1.2.5", "1.3.0", "1.3.1", "2.0.0"},
			}}
			sc := newTestScanner(reg, &stubStore{})

			results, errs := sc.Scan(context.Background(), []ContainerInfo{
				{ExternalID: "ctr1", Name: "db", Image: "postgres:1.2.3", Labels: tc.labels},
			})
			require.Empty(t, errs)
			require.Len(t, results, 1)
			assert.Equal(t, tc.wantTag, results[0].LatestTag)
			assert.Equal(t, tc.wantType, results[0].UpdateType)
		})
	}
}

func TestScanner_TrackLabels_NothingInsideTheTrackedLevel(t *testing.T) {
	reg := &stubRegistry{tags: map[string][]string{"library/postgres": {"1.2.3", "2.0.0"}}}
	sc := newTestScanner(reg, &stubStore{})

	results, errs := sc.Scan(context.Background(), []ContainerInfo{{
		ExternalID: "ctr1", Name: "db", Image: "postgres:1.2.3",
		Labels: map[string]string{"maintenant.update.ignore_major": "true"},
	}})
	require.Empty(t, errs)
	assert.Empty(t, results)
}

// A floating tag whose next line is out of the tracked level still gets its digest compared.
func TestScanner_TrackMinor_FloatingTagFallsBackToDigest(t *testing.T) {
	reg := &stubRegistry{
		tags:   map[string][]string{"library/traefik": {"v3", "v3.7.10", "v4"}},
		digest: "sha256:new",
	}
	sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "v3")})

	results, errs := sc.Scan(context.Background(), []ContainerInfo{{
		ExternalID: "ctr1", Name: "proxy", Image: "traefik:v3",
		Labels: map[string]string{"maintenant.update.track": "minor"},
	}})
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "v3", results[0].LatestTag)
	assert.Equal(t, UpdateTypeDigestOnly, results[0].UpdateType)
}

func TestScanner_DigestOnlyLabels_NeverListTags(t *testing.T) {
	for _, labels := range []map[string]string{
		{"maintenant.update.digest_only": "true"},
		{"maintenant.update.track": "digest"},
	} {
		reg := &stubRegistry{
			tags:   map[string][]string{"library/postgres": {"16.4.0", "17.0.0"}},
			digest: "sha256:new",
		}
		sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "16.4.0")})

		results, errs := sc.Scan(context.Background(), []ContainerInfo{
			{ExternalID: "ctr1", Name: "db", Image: "postgres:16.4.0", Labels: labels},
		})
		require.Empty(t, errs)
		require.Len(t, results, 1, "labels %v", labels)
		assert.Equal(t, "16.4.0", results[0].LatestTag)
		assert.Equal(t, UpdateTypeDigestOnly, results[0].UpdateType)
		assert.Zero(t, reg.listCalls, "digest-only mode must not read the tag list")
	}
}

func TestScanner_DigestOnlyLabel_UnchangedDigestIsNoUpdate(t *testing.T) {
	reg := &stubRegistry{digest: "sha256:old"}
	sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "16.4.0")})

	results, errs := sc.Scan(context.Background(), []ContainerInfo{{
		ExternalID: "ctr1", Name: "db", Image: "postgres:16.4.0",
		Labels: map[string]string{"maintenant.update.digest_only": "true"},
	}})
	require.Empty(t, errs)
	assert.Empty(t, results)
}

func TestScanner_ImageExclusion_ComparesTheNameWithoutTag(t *testing.T) {
	cases := []struct {
		pattern  string
		image    string
		excluded bool
	}{
		{"myregistry.example.com/internal-app", "myregistry.example.com/internal-app:1.2.0", true},
		{"myregistry.example.com/internal-app", "myregistry.example.com/internal-app:1.2.0@sha256:abc", true},
		{"myregistry.example.com/*", "myregistry.example.com/internal-app:1.2.0", true},
		{"nginx", "docker.io/library/nginx:1.2.0", true},
		{"nginx:1.*", "nginx:1.2.0", true},
		{"myregistry.example.com/internal", "myregistry.example.com/internal-app:1.2.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+" vs "+tc.image, func(t *testing.T) {
			reg := &stubRegistry{tags: map[string][]string{
				"myregistry.example.com/internal-app": {"1.2.0", "1.3.0"},
				"library/nginx":                       {"1.2.0", "1.3.0"},
			}}
			sc := newTestScanner(reg, &stubStore{exclusions: []*UpdateExclusion{
				{Pattern: tc.pattern, PatternType: ExclusionTypeImage},
			}})

			results, errs := sc.Scan(context.Background(), []ContainerInfo{
				{ExternalID: "ctr1", Name: "app", Image: tc.image},
			})
			require.Empty(t, errs)
			if tc.excluded {
				assert.Empty(t, results)
			} else {
				assert.Len(t, results, 1)
			}
		})
	}
}

// A tag filter that leaves out the current partial tag must not stop its digest comparison.
func TestScanner_PartialTag_FilterKeepsTheCurrentTagAsReference(t *testing.T) {
	for _, labels := range []map[string]string{
		{"maintenant.update.tag-include": `^v3\.\d+\.\d+$`},
		{"maintenant.update.tag-exclude": `^v3$`},
	} {
		reg := &stubRegistry{
			tags:   map[string][]string{"library/traefik": {"v3", "v3.7.9", "v3.7.10"}},
			digest: "sha256:new",
		}
		sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "v3")})

		results, errs := sc.Scan(context.Background(), []ContainerInfo{
			{ExternalID: "ctr1", Name: "proxy", Image: "traefik:v3", Labels: labels},
		})
		require.Empty(t, errs)
		require.Len(t, results, 1, "labels %v", labels)
		assert.Equal(t, "v3", results[0].LatestTag)
		assert.Equal(t, UpdateTypeDigestOnly, results[0].UpdateType)
	}
}
