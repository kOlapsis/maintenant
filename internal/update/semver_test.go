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
			name:       "same version different variant is not an update",
			currentTag: "18.3-alpine",
			allTags:    []string{"18.3", "18.3-alpine", "18.2", "18.2-alpine"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
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
			name:       "no update available same variant",
			currentTag: "1.25-alpine",
			allTags:    []string{"1.25-alpine", "1.25", "1.24-alpine"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
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
			name:       "slim-bookworm variant",
			currentTag: "3.19-slim-bookworm",
			allTags:    []string{"3.19-slim-bookworm", "3.20-slim-bookworm", "3.20-bookworm", "3.20"},
			wantTag:    "3.20-slim-bookworm",
			wantType:   UpdateTypeMinor,
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
