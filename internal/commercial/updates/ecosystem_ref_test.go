// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteLabelRef(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name  string
		image string
		tag   string
		want  string
	}{
		{"image with a tag", "nginx:1.25", "1.25", "nginx:1.25"},
		{"image without a tag", "nginx", "latest", "nginx:latest"},
		{"image without a tag and no known tag", "ghcr.io/acme/api", "", "ghcr.io/acme/api:latest"},
		{"repository and tag given apart", "ghcr.io/acme/api", "v2", "ghcr.io/acme/api:v2"},
		{"image with a tag and a digest", "nginx:1.25@" + digest, "1.25", "nginx@" + digest},
		{"image with a digest only", "ghcr.io/acme/api@" + digest, "latest", "ghcr.io/acme/api@" + digest},
		{"registry host with a port and a tag", "registry.local:5000/acme/api:1.2", "1.2", "registry.local:5000/acme/api:1.2"},
		{"registry host with a port, no tag", "registry.local:5000/acme/api", "latest", "registry.local:5000/acme/api:latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remoteLabelRef(tt.image, tt.tag)

			assert.Equal(t, tt.want, got)
			_, err := name.ParseReference(got)
			require.NoError(t, err)
		})
	}
}
