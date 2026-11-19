// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitVariant_OpenSuffixes(t *testing.T) {
	tests := []struct {
		tag     string
		version string
		variant string
	}{
		{"3.14.2-slim", "3.14.2", "-slim"},
		{"8.3-fpm", "8.3", "-fpm"},
		{"8.3-fpm-alpine", "8.3", "-fpm-alpine"},
		{"1.27-alpine3.22", "1.27", "-alpine3.22"},
		{"12-trixie", "12", "-trixie"},
		{"1.2.3-rc1", "1.2.3-rc1", ""},
		{"1.2.3-RC.2", "1.2.3-RC.2", ""},
		{"1.2.3-beta", "1.2.3-beta", ""},
		{"1.2.3-alpha2", "1.2.3-alpha2", ""},
		{"1.2.3-pre", "1.2.3-pre", ""},
		{"1.2.3-dev", "1.2.3-dev", ""},
		{"1.2.3-rc1-alpine", "1.2.3-rc1", "-alpine"},
		{"1.2.3-20240101", "1.2.3-20240101", ""},
		{"1.2.3-development", "1.2.3", "-development"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			version, variant := splitVariant(tt.tag)
			assert.Equal(t, tt.version, version)
			assert.Equal(t, tt.variant, variant)
		})
	}
}

func TestFindBestUpdate_UnlistedVariants(t *testing.T) {
	tests := []struct {
		name       string
		currentTag string
		allTags    []string
		wantTag    string
		wantType   UpdateType
	}{
		{
			name:       "slim stays slim",
			currentTag: "3.14.2-slim",
			allTags:    []string{"3.14.2-slim", "3.14.2", "3.15.0", "3.15.0-slim", "3.15.1", "3.15.1-slim", "3.16.0-rc1-slim", "3.15.1-alpine"},
			wantTag:    "3.15.1-slim",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "fpm stays fpm",
			currentTag: "8.3-fpm",
			allTags:    []string{"8.3-fpm", "8.4-fpm", "8.5-fpm", "8.5", "8.5-fpm-alpine"},
			wantTag:    "8.5-fpm",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "alpine with point release stays on the same alpine",
			currentTag: "1.27-alpine3.22",
			allTags:    []string{"1.27-alpine3.22", "1.28-alpine3.22", "1.28-alpine3.21", "1.28", "1.28-alpine"},
			wantTag:    "1.28-alpine3.22",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "trixie stays trixie",
			currentTag: "12-trixie",
			allTags:    []string{"12-trixie", "17-trixie", "17", "17-bookworm"},
			wantTag:    "17-trixie",
			wantType:   UpdateTypeMajor,
		},
		{
			name:       "pinned slim ignores other variants",
			currentTag: "3.14.2-slim",
			allTags:    []string{"3.14.2-slim", "3.15.1", "3.15.1-alpine"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
		{
			name:       "plain tag is never offered a variant tag",
			currentTag: "3.14.2",
			allTags:    []string{"3.14.2", "3.15.1-slim", "3.15.1-fpm", "3.15.1-trixie"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
		{
			name:       "plain tag still gets plain update",
			currentTag: "3.14.2",
			allTags:    []string{"3.14.2", "3.15.0", "3.15.1-slim", "3.15.1-rc1"},
			wantTag:    "3.15.0",
			wantType:   UpdateTypeMinor,
		},
		{
			name:       "prerelease is not a candidate",
			currentTag: "1.2.3",
			allTags:    []string{"1.2.3", "1.2.4-rc1", "1.3.0-beta2", "1.2.4-dev"},
			wantTag:    "",
			wantType:   UpdateTypeUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotType := findBestUpdate(tt.currentTag, tt.allTags, TrackMajor)
			assert.Equal(t, tt.wantTag, gotTag)
			assert.Equal(t, tt.wantType, gotType)
		})
	}
}
