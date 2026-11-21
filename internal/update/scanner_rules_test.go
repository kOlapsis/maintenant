// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"errors"
	"strings"
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
		name      string
		labels    map[string]string
		wantTag   string
		wantType  UpdateType
		wantMajor string
	}{
		{"no label", nil, "1.3.1", UpdateTypeMinor, "2.0.0"},
		{"track major", map[string]string{"maintenant.update.track": "major"}, "1.3.1", UpdateTypeMinor, "2.0.0"},
		{"track minor", map[string]string{"maintenant.update.track": "minor"}, "1.3.1", UpdateTypeMinor, ""},
		{"track patch", map[string]string{"maintenant.update.track": "patch"}, "1.2.5", UpdateTypePatch, ""},
		{"ignore_major", map[string]string{"maintenant.update.ignore_major": "true"}, "1.3.1", UpdateTypeMinor, ""},
		{"ignore_major with track patch", map[string]string{
			"maintenant.update.ignore_major": "true",
			"maintenant.update.track":        "patch",
		}, "1.2.5", UpdateTypePatch, ""},
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
			assert.Equal(t, tc.wantMajor, results[0].MajorTag)
		})
	}
}

func TestScanner_NewMajor_TargetsTheCurrentMajorFirst(t *testing.T) {
	cases := []struct {
		name      string
		image     string
		tags      []string
		labels    map[string]string
		baseline  string
		wantTag   string
		wantType  UpdateType
		wantMajor string
	}{
		{"three-part tag", "postgres:1.2.3", []string{"1.2.3", "1.2.5", "2.0.0"}, nil, "", "1.2.5", UpdateTypePatch, "2.0.0"},
		{"two-part tag", "postgres:18.7", []string{"18.7", "18.8", "19.0"}, nil, "sha256:new", "18.8", UpdateTypeMinor, "19.0"},
		{"two-part tag, current one unchanged", "postgres:18.7", []string{"18.7", "19.0"}, nil, "sha256:new", "19.0", UpdateTypeMajor, ""},
		{"two-part tag, current one republished", "postgres:18.7", []string{"18.7", "19.0"}, nil, "sha256:old", "18.7", UpdateTypeDigestOnly, "19.0"},
		{"track minor", "postgres:18.7", []string{"18.7", "18.8", "19.0"}, map[string]string{"maintenant.update.track": "minor"}, "sha256:new", "18.8", UpdateTypeMinor, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &stubRegistry{tags: map[string][]string{"library/postgres": tc.tags}, digest: "sha256:new"}
			store := &stubStore{}
			if tc.baseline != "" {
				_, tag, _ := ParseImageRef(tc.image)
				store.baseline = &DigestBaseline{ContainerID: "ctr1", Tag: tag, RemoteDigest: tc.baseline}
			}
			sc := newTestScanner(reg, store)

			results, errs := sc.Scan(context.Background(), []ContainerInfo{
				{ExternalID: "ctr1", Name: "db", Image: tc.image, Labels: tc.labels},
			})
			require.Empty(t, errs)
			require.Len(t, results, 1)
			assert.Equal(t, tc.wantTag, results[0].LatestTag)
			assert.Equal(t, tc.wantType, results[0].UpdateType)
			assert.Equal(t, tc.wantMajor, results[0].MajorTag)
		})
	}
}

type failingDigestRegistry struct {
	*stubRegistry
	failTag string
}

func (r failingDigestRegistry) ResolveDigests(ctx context.Context, imageRef string) (RemoteDigests, error) {
	if strings.HasSuffix(imageRef, ":"+r.failTag) {
		return RemoteDigests{}, errors.New("registry unavailable")
	}
	return r.stubRegistry.ResolveDigests(ctx, imageRef)
}

func TestScanner_NewMajor_FailedDigestCheckOfCurrentTagStillReportsTheMajor(t *testing.T) {
	reg := failingDigestRegistry{
		stubRegistry: &stubRegistry{tags: map[string][]string{"library/postgres": {"18.7", "19.0"}}, digest: "sha256:new"},
		failTag:      "18.7",
	}
	sc := newTestScanner(reg, &stubStore{})

	results, errs := sc.Scan(context.Background(), []ContainerInfo{
		{ExternalID: "ctr1", Name: "db", Image: "postgres:18.7"},
	})
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "19.0", results[0].LatestTag)
	assert.Equal(t, UpdateTypeMajor, results[0].UpdateType)
	assert.Empty(t, results[0].MajorTag)
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
