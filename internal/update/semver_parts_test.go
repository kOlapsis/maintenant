// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindBestUpdate_FourPartAndZeroPaddedTags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		current  string
		tags     []string
		wantTag  string
		wantType UpdateType
	}{
		{"four parts, newer build", "4.0.8.1874", []string{"4.0.8.1874", "4.0.8.1900", "4.0.8.999", "latest"}, "4.0.8.1900", UpdateTypePatch},
		{"four parts, newer patch", "4.0.8.1874", []string{"4.0.8.1874", "4.0.9.12", "4.0.8.2000"}, "4.0.9.12", UpdateTypePatch},
		{"four parts, newer minor", "4.0.8.1874", []string{"4.0.8.1874", "4.1.0.5"}, "4.1.0.5", UpdateTypeMinor},
		{"four parts, up to date", "4.0.8.1874", []string{"4.0.8.1874", "4.0.8.1000", "4.0.7"}, "", UpdateTypeUnknown},
		{"three parts against four", "4.0.8", []string{"4.0.8", "4.0.8.1874", "4.0.9"}, "4.0.9", UpdateTypePatch},
		{"zero-padded month", "2024.12.1", []string{"2024.12.1", "2025.01.1", "2024.09.3"}, "2025.01.1", UpdateTypeMajor},
		{"zero-padded current", "2024.01.5", []string{"2024.01.5", "2024.02.1", "2024.1.9"}, "2024.02.1", UpdateTypeMinor},
		{"variant kept", "4.0.8.1874-alpine", []string{"4.0.8.1900", "4.0.8.1890-alpine"}, "4.0.8.1890-alpine", UpdateTypePatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotTag, gotType := findBestUpdate(tc.current, tc.tags, TrackMajor)
			assert.Equal(t, tc.wantTag, gotTag)
			assert.Equal(t, tc.wantType, gotType)
		})
	}
}

func TestParseTag_StillRefusesWhatIsNotAVersion(t *testing.T) {
	for _, tag := range []string{"latest", "1.2.3.4.5", "1.x.3", "RELEASE.2024-01-01T00-00-00Z", "4.0.8.99999999999"} {
		_, err := ParseTag(tag)
		assert.Error(t, err, tag)
	}
}
