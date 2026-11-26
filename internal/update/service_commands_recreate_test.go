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

func TestGenerateCommands_ComposeNamesItsProject(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{
		Name: "prod-web-1", OrchestrationGroup: "prod", OrchestrationUnit: "web",
		ComposeWorkingDir: "/srv/app", RuntimeType: "docker",
	}

	c.Image = "nginx:1.24.0"
	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service web to nginx:1.26.0 in the compose file, then:\n"+
		"docker compose -p prod pull web\n"+
		"docker compose -p prod up -d web", svc.GenerateUpdateCommand(c, "1.26.0", "sha256:new"))
	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service web back to nginx@sha256:old in the compose file, then:\n"+
		"docker compose -p prod up -d web",
		svc.GenerateRollbackCommand(c, &ImageUpdate{Image: c.Image, CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old"}))

	c.Image = "nginx:latest"
	assert.Equal(t, "cd /srv/app\n"+
		"docker compose -p prod pull web\n"+
		"docker compose -p prod up -d --force-recreate web", svc.GenerateUpdateCommand(c, "latest", "sha256:new"))
	assert.Equal(t, "cd /srv/app\n"+
		"docker pull nginx@sha256:old\n"+
		"docker tag nginx@sha256:old nginx:latest\n"+
		"docker compose -p prod up -d --pull never --force-recreate web",
		svc.GenerateRollbackCommand(c, &ImageUpdate{Image: c.Image, CurrentTag: "latest", LatestTag: "latest", PreviousDigest: "sha256:old"}))

	c.OrchestrationGroup, c.ComposeWorkingDir = "my app; id", ""
	assert.Equal(t, "cd <compose-project-dir>\n"+
		"docker compose -p 'my app; id' pull web\n"+
		"docker compose -p 'my app; id' up -d --force-recreate web", svc.GenerateUpdateCommand(c, "latest", "sha256:new"))
}

func TestGenerateCommands_StandaloneQuotesTheNameInTheCommentToo(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{Name: "web; id", Image: "nginx:1.24.0", RuntimeType: "docker"}

	assert.Equal(t, "docker pull nginx:1.26.0\n"+
		"# Recreate 'web; id' with the options it was started with, using nginx:1.26.0",
		svc.GenerateUpdateCommand(c, "1.26.0", "sha256:new"))
}
