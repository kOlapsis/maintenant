// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyImageLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   Container
	}{
		{
			name: "oci labels",
			labels: map[string]string{
				"org.opencontainers.image.version":     "1.4.2",
				"org.opencontainers.image.source":      "https://github.com/acme/app",
				"org.opencontainers.image.url":         "https://acme.dev",
				"org.opencontainers.image.description": "Acme app",
			},
			want: Container{ImageVersion: "1.4.2", ImageSource: "https://github.com/acme/app", ImageURL: "https://acme.dev", ImageDescription: "Acme app"},
		},
		{
			name: "label-schema fallback",
			labels: map[string]string{
				"org.label-schema.version":     "2.0",
				"org.label-schema.vcs-url":     "https://git.example.com/app.git",
				"org.label-schema.url":         "http://example.com",
				"org.label-schema.description": "Legacy",
			},
			want: Container{ImageVersion: "2.0", ImageSource: "https://git.example.com/app.git", ImageURL: "http://example.com", ImageDescription: "Legacy"},
		},
		{
			name: "documentation fallback for url",
			labels: map[string]string{
				"org.opencontainers.image.documentation": "https://docs.acme.dev",
			},
			want: Container{ImageURL: "https://docs.acme.dev"},
		},
		{
			name: "non-web source and url are dropped",
			labels: map[string]string{
				"org.opencontainers.image.source": "git@github.com:acme/app.git",
				"org.label-schema.vcs-url":        "https://github.com/acme/app",
				"org.opencontainers.image.url":    "javascript:alert(1)",
			},
			want: Container{ImageSource: "https://github.com/acme/app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Container
			assert.True(t, c.ApplyImageLabels(tt.labels))
			assert.Equal(t, tt.want, c)
			assert.False(t, c.ApplyImageLabels(tt.labels), "a second pass changes nothing")
		})
	}
}
