// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C6: every value a label can set is quoted for a POSIX shell.

func composeService(service, dir string) ContainerInfo {
	return ContainerInfo{
		Name:               "app-" + service + "-1",
		Image:              "nginx:latest",
		OrchestrationGroup: "app",
		OrchestrationUnit:  service,
		ComposeWorkingDir:  dir,
		RuntimeType:        "docker",
	}
}

// US1 scenario 1, FR-001: a service name with a `;` and a pipe is quoted, and no part of it is outside the quotes.
func TestGenerateUpdateCommand_QuotesAHostileComposeServiceName(t *testing.T) {
	svc := &Service{}
	got := svc.GenerateUpdateCommand(composeService("web; curl x | sh #", "/srv/app"), "latest", "sha256:new")

	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull 'web; curl x | sh #'\n"+
		"docker compose up -d --force-recreate 'web; curl x | sh #'", got)
	assert.Equal(t, strings.Count(got, "web; curl x | sh #"), strings.Count(got, "'web; curl x | sh #'"),
		"no occurrence of the value may sit outside single quotes")

	other := svc.GenerateUpdateCommand(composeService("api; wget y | sh", "/srv/api"), "latest", "sha256:new")
	assert.Equal(t, "cd /srv/api\n"+
		"docker compose pull 'api; wget y | sh'\n"+
		"docker compose up -d --force-recreate 'api; wget y | sh'", other)
}

// US1 scenario 2, FR-001: a command substitution is quoted.
func TestGenerateUpdateCommand_QuotesACommandSubstitution(t *testing.T) {
	svc := &Service{}

	got := svc.GenerateUpdateCommand(composeService("$(id)", "/srv/app"), "latest", "sha256:new")
	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull '$(id)'\n"+
		"docker compose up -d --force-recreate '$(id)'", got)

	got = svc.GenerateUpdateCommand(composeService("`whoami`", "/srv/app"), "latest", "sha256:new")
	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull '`whoami`'\n"+
		"docker compose up -d --force-recreate '`whoami`'", got)
}

// US1 scenario 3, FR-001: an apostrophe is escaped so the shell reads the value back unchanged.
func TestGenerateUpdateCommand_EscapesAnApostrophe(t *testing.T) {
	svc := &Service{}

	got := svc.GenerateUpdateCommand(composeService("it's", "/srv/app"), "latest", "sha256:new")
	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull 'it'\\''s'\n"+
		"docker compose up -d --force-recreate 'it'\\''s'", got)

	got = svc.GenerateUpdateCommand(composeService("o'neil's", "/srv/app"), "latest", "sha256:new")
	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull 'o'\\''neil'\\''s'\n"+
		"docker compose up -d --force-recreate 'o'\\''neil'\\''s'", got)
}

// US1 scenario 1 and 4 together: a working directory with a space is quoted, as in the contract example.
func TestGenerateUpdateCommand_QuotesAComposeDirWithASpace(t *testing.T) {
	svc := &Service{}
	got := svc.GenerateUpdateCommand(composeService("web; curl x | sh #", "/srv/my app"), "latest", "sha256:new")

	assert.Equal(t, "cd '/srv/my app'\n"+
		"docker compose pull 'web; curl x | sh #'\n"+
		"docker compose up -d --force-recreate 'web; curl x | sh #'", got)
}

// US1 scenario 4, FR-002: a Compose directory that is not a clean absolute path is replaced by the marker.
func TestGenerateCommands_UncleanComposeDirBecomesTheMarker(t *testing.T) {
	svc := &Service{}
	for name, dir := range map[string]string{
		"relative":     "app",
		"empty":        "",
		"dotdot":       "/srv/../etc",
		"trailing":     "/srv/app/",
		"control":      "/srv/app\x01",
		"newline":      "/srv/app\n",
		"dot-relative": "./app",
	} {
		t.Run(name, func(t *testing.T) {
			c := composeService("web", dir)
			update := svc.GenerateUpdateCommand(c, "latest", "sha256:new")
			assert.True(t, strings.HasPrefix(update, "cd <compose-project-dir>\n"), "update: %q", update)
			if dir != "" {
				assert.NotContains(t, update, dir)
			}

			rollback := svc.GenerateRollbackCommand(c, &ImageUpdate{
				Image: "nginx:latest", CurrentTag: "latest", LatestTag: "latest", PreviousDigest: "sha256:old",
			})
			assert.True(t, strings.HasPrefix(rollback, "cd <compose-project-dir>\n"), "rollback: %q", rollback)
		})
	}
}

// US1 scenario 5, FR-003: a control character in any interpolated value produces no command at all.
func TestGenerateCommands_ControlCharacterProducesNoCommand(t *testing.T) {
	svc := &Service{}
	controls := map[string]string{"newline": "\n", "cr": "\r", "tab": "\t", "esc": "\x1b", "del": "\x7f"}

	compose := func(service string) ContainerInfo { return composeService(service, "/srv/app") }
	kube := func(unit, group, kind, pod string) ContainerInfo {
		return ContainerInfo{
			Name: "api", Image: "ghcr.io/acme/api:1.0.0", RuntimeType: "kubernetes",
			ControllerKind: kind, OrchestrationUnit: unit, OrchestrationGroup: group, PodContainer: pod,
		}
	}
	swarm := func(service string) ContainerInfo {
		return ContainerInfo{
			Name: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.24.0", OrchestrationGroup: "shop",
			RuntimeType: "docker", ControllerKind: "swarm-service", SwarmService: service,
		}
	}
	standalone := func(name string) ContainerInfo {
		return ContainerInfo{Name: name, Image: "nginx:1.24.0", RuntimeType: "docker"}
	}

	for ctrlName, ctrl := range controls {
		kubeNamed := kube("api", "prod", "Deployment", "")
		kubeNamed.Name = "api" + ctrl
		cases := map[string]ContainerInfo{
			"compose service":   compose("web" + ctrl + "x"),
			"kube name as pod":  kubeNamed,
			"kube namespace":    kube("api", "prod"+ctrl+"x", "Deployment", "server"),
			"kube unit":         kube("api"+ctrl, "prod", "Deployment", "server"),
			"kube kind":         kube("api", "prod", "Deployment"+ctrl, "server"),
			"swarm service":     swarm("shop_web" + ctrl),
			"standalone name":   standalone("web" + ctrl + "x"),
			"compose project":   {Name: "app-web-1", Image: "nginx:1.24.0", RuntimeType: "docker", OrchestrationGroup: "app" + ctrl, OrchestrationUnit: "web", ComposeWorkingDir: "/srv/app"},
			"kube pod container": kube("api", "prod", "Deployment", "server"+ctrl+"x"),
		}
		for caseName, c := range cases {
			t.Run(ctrlName+"/"+caseName, func(t *testing.T) {
				assert.Empty(t, svc.GenerateUpdateCommand(c, "1.26.0", "sha256:new"), "update")
				assert.Empty(t, svc.GenerateUpdateCommand(c, "1.24.0", "sha256:new"), "update same tag")
				assert.Empty(t, svc.GenerateRollbackCommand(c, &ImageUpdate{
					Image: c.Image, CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old",
				}), "rollback")
				assert.Empty(t, svc.GenerateFixCommand(c, "1.24.0", "1.25.0"), "fix")
			})
		}
	}
}

// US1 scenario 6, FR-004, SC-002: ordinary values give the commands of today, byte for byte, with no quote.
func TestGenerateCommands_OrdinaryValuesAreUnchanged(t *testing.T) {
	svc := &Service{}

	compose := ContainerInfo{
		Name: "app-web-1", Image: "nginx:latest", OrchestrationGroup: "app", OrchestrationUnit: "web",
		ComposeWorkingDir: "/srv/app", RuntimeType: "docker",
	}
	assert.Equal(t, "cd /srv/app\ndocker compose pull web\ndocker compose up -d --force-recreate web",
		svc.GenerateUpdateCommand(compose, "latest", "sha256:new"))

	kube := ContainerInfo{
		Name: "api", Image: "ghcr.io/acme/api:1.2.3@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RuntimeType: "kubernetes", ControllerKind: "Deployment", OrchestrationUnit: "api",
		OrchestrationGroup: "prod", PodContainer: "server",
	}
	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api:1.2.3@sha256:new -n prod",
		svc.GenerateUpdateCommand(kube, "1.2.3", "sha256:new"))
	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api@sha256:old -n prod",
		svc.GenerateRollbackCommand(kube, &ImageUpdate{Image: kube.Image, CurrentTag: "1.2.3", LatestTag: "1.2.3", PreviousDigest: "sha256:old"}))

	swarm := ContainerInfo{
		Name: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.24.0", OrchestrationGroup: "shop",
		RuntimeType: "docker", ControllerKind: "swarm-service", SwarmService: "shop_web",
	}
	assert.Equal(t, "docker service update --image nginx:1.26.0 shop_web", svc.GenerateUpdateCommand(swarm, "1.26.0", "sha256:new"))

	standalone := ContainerInfo{Name: "web-1.prod", Image: "ghcr.io/acme/web:2.1.0", RuntimeType: "docker"}
	assert.Equal(t, "docker pull ghcr.io/acme/web:2.2.0\n"+
		"docker stop web-1.prod && docker rm web-1.prod\n"+
		"docker run -d --name web-1.prod ghcr.io/acme/web:2.2.0", svc.GenerateUpdateCommand(standalone, "2.2.0", "sha256:new"))

	for _, c := range []ContainerInfo{compose, kube, swarm, standalone} {
		assert.NotContains(t, svc.GenerateUpdateCommand(c, "2.2.0", "sha256:new"), "'", "%s", c.Name)
	}
}

// US1 scenario 7, FR-001: Kubernetes and Swarm commands quote each interpolated value separately.
func TestGenerateCommands_KubernetesAndSwarmValuesAreQuoted(t *testing.T) {
	svc := &Service{}

	kube := ContainerInfo{
		Name: "api", Image: "ghcr.io/acme/api:1.0.0", RuntimeType: "kubernetes",
		ControllerKind: "Deployment", OrchestrationUnit: "api; id", OrchestrationGroup: "prod $(id)", PodContainer: "server it",
	}
	assert.Equal(t, "kubectl set image deployment/'api; id' 'server it'=ghcr.io/acme/api:1.1.0 -n 'prod $(id)'",
		svc.GenerateUpdateCommand(kube, "1.1.0", "sha256:new"))
	assert.Equal(t, "kubectl set image deployment/'api; id' 'server it'=ghcr.io/acme/api@sha256:old -n 'prod $(id)'",
		svc.GenerateRollbackCommand(kube, &ImageUpdate{Image: kube.Image, CurrentTag: "1.0.0", LatestTag: "1.1.0", PreviousDigest: "sha256:old"}))

	kind := ContainerInfo{
		Name: "api", Image: "ghcr.io/acme/api:1.0.0", RuntimeType: "kubernetes",
		ControllerKind: "Deployment;id", OrchestrationUnit: "api", OrchestrationGroup: "prod", PodContainer: "server",
	}
	assert.Equal(t, "kubectl set image 'deployment;id'/api server=ghcr.io/acme/api:1.1.0 -n prod",
		svc.GenerateUpdateCommand(kind, "1.1.0", "sha256:new"))

	swarm := ContainerInfo{
		Name: "shop_web.1.x7k2p9q4m1n8", Image: "nginx:1.24.0", OrchestrationGroup: "shop",
		RuntimeType: "docker", ControllerKind: "swarm-service", SwarmService: "shop_web; rm -rf /",
	}
	assert.Equal(t, "docker service update --image nginx:1.26.0 'shop_web; rm -rf /'",
		svc.GenerateUpdateCommand(swarm, "1.26.0", "sha256:new"))
	assert.Equal(t, "docker service update --image nginx@sha256:old 'shop_web; rm -rf /'",
		svc.GenerateRollbackCommand(swarm, &ImageUpdate{Image: swarm.Image, CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old"}))
	assert.Equal(t, "docker service update --image nginx:1.25.0 'shop_web; rm -rf /'",
		svc.GenerateFixCommand(swarm, "1.24.0", "1.25.0"))
}

// Edge case: the container name comes from the runtime, not a label, but it is quoted like the rest.
func TestGenerateCommands_StandaloneContainerNameIsQuoted(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{Name: "web; id", Image: "ghcr.io/acme/web:2.1.0", RuntimeType: "docker"}

	assert.Equal(t, "docker pull ghcr.io/acme/web:2.2.0\n"+
		"docker stop 'web; id' && docker rm 'web; id'\n"+
		"docker run -d --name 'web; id' ghcr.io/acme/web:2.2.0", svc.GenerateUpdateCommand(c, "2.2.0", "sha256:new"))
	assert.Equal(t, "docker pull ghcr.io/acme/web@sha256:old\n"+
		"docker stop 'web; id' && docker rm 'web; id'\n"+
		"docker run -d --name 'web; id' ghcr.io/acme/web@sha256:old",
		svc.GenerateRollbackCommand(c, &ImageUpdate{Image: c.Image, CurrentTag: "2.1.0", LatestTag: "2.2.0", PreviousDigest: "sha256:old"}))
}

// FR-001 on the rollback and fix paths of a Compose service, the comment line included.
func TestGenerateRollbackAndFixCommands_ComposeServiceIsQuotedEverywhere(t *testing.T) {
	svc := &Service{}
	c := composeService("web it", "/srv/app")
	c.Image = "nginx:1.24.0"

	rollback := svc.GenerateRollbackCommand(c, &ImageUpdate{Image: "nginx:1.24.0", CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old"})
	require.NotEmpty(t, rollback)
	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service 'web it' back to nginx@sha256:old in the compose file, then:\n"+
		"docker compose up -d 'web it'", rollback)

	fix := svc.GenerateFixCommand(c, "1.24.0", "1.25.0")
	require.NotEmpty(t, fix)
	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service 'web it' to nginx:1.25.0 in the compose file, then:\n"+
		"docker compose pull 'web it'\n"+
		"docker compose up -d 'web it'", fix)
}
