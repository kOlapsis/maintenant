// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateFixCommand_WritesTheFixLikeTheRunningTag(t *testing.T) {
	svc := &Service{}
	for _, tc := range []struct {
		name, current, fixed, wantRef string
	}{
		{"variant kept", "1.25.3-alpine", "1.25.4", "nginx:1.25.4-alpine"},
		{"v prefix kept", "v2.10.0", "2.11.3", "nginx:v2.11.3"},
		{"v prefix of the fix dropped", "2.10.0", "v2.11.3", "nginx:2.11.3"},
		{"same version in another variant", "1.25.3-alpine", "1.25.3", ""},
		{"distribution package version", "1.22.0", "1.22.1-9+deb12u1", ""},
		{"pre-release fix", "1.22.0", "1.23.0-rc.1", ""},
		{"commit hash", "1.22.0", "3f2a9c1", ""},
		{"downgrade", "1.25.4", "1.25.3", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := ContainerInfo{Name: "web", Image: "nginx:" + tc.current, RuntimeType: "docker", SwarmService: "web"}
			want := ""
			if tc.wantRef != "" {
				want = "docker service update --image " + tc.wantRef + " web"
			}
			assert.Equal(t, want, svc.GenerateFixCommand(c, tc.current, tc.fixed))
		})
	}
}

func TestIsFixedByUpdate_IgnoresTheVariant(t *testing.T) {
	assert.True(t, IsFixedByUpdate("1.27.0-alpine", "1.27.0"))
	assert.True(t, IsFixedByUpdate("v1.27.1-alpine3.20", "1.27.0"))
	assert.False(t, IsFixedByUpdate("1.26.9-alpine", "1.27.0"))
	assert.False(t, IsFixedByUpdate("latest", "1.27.0"))
}
