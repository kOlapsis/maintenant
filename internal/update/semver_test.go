package update

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitVariant(t *testing.T) {
	tests := []struct {
		tag     string
		version string
		variant string
	}{
		{"18.3", "18.3", ""},
		{"18.3-alpine", "18.3", "-alpine"},
		{"1.25-alpine3.20", "1.25", "-alpine3.20"},
		{"16-bookworm", "16", "-bookworm"},
		{"3.19.1-slim-bullseye", "3.19.1", "-slim-bullseye"},
		{"latest", "latest", ""},
		{"alpine", "alpine", ""},
		{"1.0-noble", "1.0", "-noble"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			version, variant := splitVariant(tt.tag)
			assert.Equal(t, tt.version, version)
			assert.Equal(t, tt.variant, variant)
		})
	}
}

func TestFindBestUpdate_VariantSuffix(t *testing.T) {
	tests := []struct {
		name       string
		currentTag string
		allTags    []string
		wantTag    string
		wantType   UpdateType
	}{
		{
			name:       "same version different variant falls back to digest comparison",
			currentTag: "18.3-alpine",
			allTags:    []string{"18.3", "18.3-alpine", "18.2", "18.2-alpine"},
			wantTag:    "18.3-alpine",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "newer alpine version is an update",
			currentTag: "18.2-alpine",
			allTags:    []string{"18.3", "18.3-alpine", "18.2", "18.2-alpine"},
			wantTag:    "18.3-alpine",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "plain tag ignores variant tags",
			currentTag: "18.2",
			allTags:    []string{"18.3", "18.3-alpine", "18.2", "18.2-alpine"},
			wantTag:    "18.3",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "no newer variant tag falls back to digest comparison",
			currentTag: "1.25-alpine",
			allTags:    []string{"1.25-alpine", "1.25", "1.24-alpine"},
			wantTag:    "1.25-alpine",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "major update with bookworm",
			currentTag: "16-bookworm",
			allTags:    []string{"16-bookworm", "17-bookworm", "17", "16"},
			wantTag:    "17-bookworm",
			wantType:   UpdateTypeMajor,
		},
		{
			name:       "major update plain tag",
			currentTag: "8.0",
			allTags:    []string{"8.0", "8.1", "9.0"},
			wantTag:    "9.0",
			wantType:   UpdateTypeMajor,
		},
		{
			name:       "non-semver tag returns same tag for digest comparison",
			currentTag: "latest",
			allTags:    []string{"latest", "1.0", "2.0"},
			wantTag:    "latest",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "non-semver lts tag returns same tag not latest",
			currentTag: "lts",
			allTags:    []string{"latest", "lts", "1.0", "2.0"},
			wantTag:    "lts",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "non-semver alpine tag returns same tag not latest",
			currentTag: "alpine",
			allTags:    []string{"latest", "alpine", "1.0-alpine", "1.0"},
			wantTag:    "alpine",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "non-semver stable tag returns same tag not latest",
			currentTag: "stable",
			allTags:    []string{"latest", "stable", "1.0"},
			wantTag:    "stable",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "non-semver tag not in registry returns empty",
			currentTag: "custom-tag",
			allTags:    []string{"latest", "1.0", "2.0"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
		{
			name:       "numeric build ID tag is not treated as version update",
			currentTag: "v1.20.1",
			allTags:    []string{"v1.20.1", "v1.20.2", "608111629", "v1.21.0"},
			wantTag:    "v1.21.0",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "numeric build ID only — no real update available",
			currentTag: "v1.20.1",
			allTags:    []string{"v1.20.1", "608111629", "123456789"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
		{
			name:       "slim-bookworm variant",
			currentTag: "3.19-slim-bookworm",
			allTags:    []string{"3.19-slim-bookworm", "3.20-slim-bookworm", "3.20-bookworm", "3.20"},
			wantTag:    "3.20-slim-bookworm",
			wantType:   UpdateTypeMinor,
		},
		{
			// Issue #62: "v3" already resolves to the newest v3.x.y, so proposing
			// v3.7.10 tells the user to install what they are already running.
			name:       "major-only tag never proposes the release it already resolves to",
			currentTag: "v3",
			allTags:    []string{"v2", "v2.11.0", "v3", "v3.7.9", "v3.7.10"},
			wantTag:    "v3",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "major-only tag proposes the next major of the same shape",
			currentTag: "v3",
			allTags:    []string{"v3", "v3.7.10", "v4", "v4.0.1"},
			wantTag:    "v4",
			wantType:   UpdateTypeMajor,
		},
		{
			name:       "major-only tag ignores candidates written without the v prefix",
			currentTag: "v3",
			allTags:    []string{"v3", "4"},
			wantTag:    "v3",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "minor-level tag never proposes its own patch releases",
			currentTag: "1.2",
			allTags:    []string{"1.2", "1.2.8", "1.2.9"},
			wantTag:    "1.2",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "minor-level tag proposes a higher minor of the same shape",
			currentTag: "1.2",
			allTags:    []string{"1.2", "1.2.9", "1.3", "1.3.4", "2.0"},
			wantTag:    "2.0",
			wantType:   UpdateTypeMajor,
		},
		{
			name:       "numeric build ID is not a major upgrade for a partial tag",
			currentTag: "3",
			allTags:    []string{"3", "3.7.10", "608111629"},
			wantTag:    "3",
			wantType:   UpdateTypeDigestOnly,
		},
		{
			name:       "partial tag no longer published returns empty",
			currentTag: "v3",
			allTags:    []string{"v3.7.10", "v4.0.0"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotType := FindBestUpdate(tt.currentTag, tt.allTags)
			assert.Equal(t, tt.wantTag, gotTag)
			assert.Equal(t, tt.wantType, gotType)
		})
	}
}

func TestSemverPrecision(t *testing.T) {
	tests := []struct {
		versionPart string
		want        int
	}{
		{"v3", 1},
		{"3", 1},
		{"1.2", 2},
		{"1.20.1", 3},
		{"v1.20.1", 3},
		{"1.2-rc1", 2},
		{"1.2.3+build.5", 3},
		{"608111629", 1},
	}
	for _, tt := range tests {
		t.Run(tt.versionPart, func(t *testing.T) {
			assert.Equal(t, tt.want, semverPrecision(tt.versionPart))
		})
	}
}

func TestClassifyUpdate(t *testing.T) {
	parse := func(s string) *semver.Version {
		v, err := ParseTag(s)
		require.NoError(t, err)
		return v
	}

	assert.Equal(t, UpdateTypeMajor, ClassifyUpdate(parse("1.0.0"), parse("2.0.0")))
	assert.Equal(t, UpdateTypeMinor, ClassifyUpdate(parse("1.0.0"), parse("1.1.0")))
	assert.Equal(t, UpdateTypePatch, ClassifyUpdate(parse("1.0.0"), parse("1.0.1")))
	assert.Equal(t, UpdateTypeUnknown, ClassifyUpdate(parse("1.0.0"), parse("1.0.0")))
}
