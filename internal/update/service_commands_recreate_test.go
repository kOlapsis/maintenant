// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCommands_StandaloneNeverRecreatesTheContainerBare(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{Name: "web", Image: "ghcr.io/acme/web:2.1.0", RuntimeType: "docker"}
	u := &ImageUpdate{Image: c.Image, CurrentTag: "2.1.0", LatestTag: "2.2.0", PreviousDigest: "sha256:old"}

	update := svc.GenerateUpdateCommand(c, "2.2.0", "sha256:new")
	assert.Equal(t, "docker pull ghcr.io/acme/web:2.2.0\n"+
		"# Recreate web with the options it was started with, using ghcr.io/acme/web:2.2.0", update)

	rollback := svc.GenerateRollbackCommand(c, u)
	assert.Equal(t, "docker pull ghcr.io/acme/web@sha256:old\n"+
		"# Recreate web with the options it was started with, using ghcr.io/acme/web@sha256:old", rollback)

	fix := svc.GenerateFixCommand(c, "2.1.0", "2.1.5")
	assert.Equal(t, "docker pull ghcr.io/acme/web:2.1.5\n"+
		"# Recreate web with the options it was started with, using ghcr.io/acme/web:2.1.5", fix)

	for _, cmd := range []string{update, rollback, fix} {
		for _, destructive := range []string{"docker stop", "docker rm", "docker run"} {
			assert.NotContains(t, cmd, destructive)
		}
	}
}

func TestGenerateCommands_StandaloneQuotesTheNameInTheCommentToo(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{Name: "web; id", Image: "nginx:1.24.0", RuntimeType: "docker"}

	assert.Equal(t, "docker pull nginx:1.26.0\n"+
		"# Recreate 'web; id' with the options it was started with, using nginx:1.26.0",
		svc.GenerateUpdateCommand(c, "1.26.0", "sha256:new"))
}
