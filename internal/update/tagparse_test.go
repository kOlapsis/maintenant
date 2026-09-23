// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTagOSVariant(t *testing.T) {
	tests := []struct {
		tag     string
		wantEco string
		wantOK  bool
	}{
		// Full tag = OS variant
		{"alpine", "Alpine:3.20", true},
		{"alpine3.19", "Alpine:3.19", true},
		{"bookworm", "Debian:12", true},
		{"bookworm-slim", "Debian:12", true},
		{"bullseye", "Debian:11", true},
		{"bullseye-slim", "Debian:11", true},
		{"noble", "Ubuntu:24.04", true},
		{"jammy", "Ubuntu:22.04", true},
		{"focal", "Ubuntu:20.04", true},

		// Suffix variants
		{"1.25-alpine", "Alpine:3.20", true},
		{"1.25-alpine3.19", "Alpine:3.19", true},
		{"16-bookworm", "Debian:12", true},
		{"3.2-slim-bookworm", "Debian:12", true},
		{"8.0-bullseye", "Debian:11", true},
		{"3.19-slim-bullseye", "Debian:11", true},
		{"22.04-jammy", "Ubuntu:22.04", true},
		{"24.04-noble", "Ubuntu:24.04", true},

		// No OS variant
		{"latest", "", false},
		{"1.25", "", false},
		{"16.3", "", false},
		{"", "", false},
		{"slim", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			eco, ok := ParseTagOSVariant(tt.tag)
			assert.Equal(t, tt.wantOK, ok, "ok mismatch for tag %q", tt.tag)
			if ok {
				assert.Equal(t, tt.wantEco, eco, "ecosystem mismatch for tag %q", tt.tag)
			}
		})
	}
}
