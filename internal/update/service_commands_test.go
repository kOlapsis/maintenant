// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/event"
)

func composeWeb(image string) ContainerInfo {
	return ContainerInfo{
		Name:               "app-web-1",
		Image:              image,
		OrchestrationGroup: "app",
		OrchestrationUnit:  "web",
		ComposeWorkingDir:  "/srv/app",
		RuntimeType:        "docker",
	}
}

func TestGenerateRollbackCommand_ComposeSameTag_RetagsThePreviousImage(t *testing.T) {
	svc := &Service{}
	c := composeWeb("nginx:latest")
	u := &ImageUpdate{Image: "nginx:latest", CurrentTag: "latest", LatestTag: "latest", PreviousDigest: "sha256:old"}

	got := svc.GenerateRollbackCommand(c, u)

	assert.Equal(t, "cd /srv/app\n"+
		"docker pull nginx@sha256:old\n"+
		"docker tag nginx@sha256:old nginx:latest\n"+
		"docker compose up -d --pull never --force-recreate web", got)
	assert.NotEqual(t, svc.GenerateUpdateCommand(c, "latest", "sha256:new"), got)
}

func TestGenerateRollbackCommand_ComposeNewTag_PinsThePreviousImage(t *testing.T) {
	svc := &Service{}
	c := composeWeb("nginx:1.24.0")

	byDigest := svc.GenerateRollbackCommand(c, &ImageUpdate{
		Image: "nginx:1.24.0", CurrentTag: "1.24.0", LatestTag: "1.26.0", PreviousDigest: "sha256:old",
	})
	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service web back to nginx@sha256:old in the compose file, then:\n"+
		"docker compose up -d web", byDigest)

	byTag := svc.GenerateRollbackCommand(c, &ImageUpdate{
		Image: "nginx:1.24.0", CurrentTag: "1.24.0", LatestTag: "1.26.0",
	})
	assert.Contains(t, byTag, "back to nginx:1.24.0 in the compose file")
}

func TestGenerateRollbackCommand_Standalone(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{Name: "web", Image: "ghcr.io/acme/web:2.1.0", RuntimeType: "docker"}

	got := svc.GenerateRollbackCommand(c, &ImageUpdate{
		Image: "ghcr.io/acme/web:2.1.0", CurrentTag: "2.1.0", LatestTag: "2.2.0", PreviousDigest: "sha256:old",
	})

	assert.Equal(t, "docker pull ghcr.io/acme/web@sha256:old\n"+
		"docker stop web && docker rm web\n"+
		"docker run -d --name web ghcr.io/acme/web@sha256:old", got)
}

func TestGenerateRollbackCommand_Kubernetes(t *testing.T) {
	svc := &Service{}
	c := ContainerInfo{
		Name: "api", Image: "ghcr.io/acme/api:1.0.0", RuntimeType: "kubernetes",
		ControllerKind: "Deployment", OrchestrationUnit: "api", OrchestrationGroup: "prod",
	}

	got := svc.GenerateRollbackCommand(c, &ImageUpdate{
		Image: "ghcr.io/acme/api:1.0.0", CurrentTag: "1.0.0", LatestTag: "1.1.0",
	})

	assert.Equal(t, "kubectl set image deployment/api api=ghcr.io/acme/api:1.0.0 -n prod", got)
}

func kubeAPI(image string) ContainerInfo {
	return ContainerInfo{
		Name: "api", Image: image, RuntimeType: "kubernetes", ControllerKind: "Deployment",
		OrchestrationUnit: "api", OrchestrationGroup: "prod", PodContainer: "server",
	}
}

// Setting the image a workload already has changes nothing, so a republished tag is deployed by its digest.
func TestGenerateUpdateCommand_KubernetesSameTagDeploysTheNewDigest(t *testing.T) {
	svc := &Service{}

	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api:stable@sha256:new -n prod",
		svc.GenerateUpdateCommand(kubeAPI("ghcr.io/acme/api:stable"), "stable", "sha256:new"))
	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api:stable@sha256:new -n prod",
		svc.GenerateUpdateCommand(kubeAPI("ghcr.io/acme/api:stable@sha256:old"), "stable", "sha256:new"))
	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api:1.1.0 -n prod",
		svc.GenerateUpdateCommand(kubeAPI("ghcr.io/acme/api:1.0.0"), "1.1.0", "sha256:new"))
}

func TestGenerateRollbackCommand_KubernetesNamesThePodContainer(t *testing.T) {
	svc := &Service{}

	got := svc.GenerateRollbackCommand(kubeAPI("ghcr.io/acme/api:stable"), &ImageUpdate{
		Image: "ghcr.io/acme/api:stable", CurrentTag: "stable", LatestTag: "stable", PreviousDigest: "sha256:old",
	})

	assert.Equal(t, "kubectl set image deployment/api server=ghcr.io/acme/api@sha256:old -n prod", got)
}

// A moving tag without a known digest cannot name the image it pointed to before.
func TestGenerateRollbackCommand_MovingTagWithoutDigest(t *testing.T) {
	svc := &Service{}
	for _, tag := range []string{"latest", "v3", "1.2"} {
		got := svc.GenerateRollbackCommand(composeWeb("traefik:"+tag), &ImageUpdate{
			Image: "traefik:" + tag, CurrentTag: tag, LatestTag: tag,
		})
		assert.Empty(t, got, tag)
	}
}

func TestGenerateUpdateCommand_ComposeNewTagIsWrittenInTheComposeFile(t *testing.T) {
	svc := &Service{}

	assert.Equal(t, "cd /srv/app\n"+
		"# Set the image of service web to nginx:1.26.0 in the compose file, then:\n"+
		"docker compose pull web\n"+
		"docker compose up -d web", svc.GenerateUpdateCommand(composeWeb("nginx:1.24.0"), "1.26.0", "sha256:new"))

	assert.Equal(t, "cd /srv/app\n"+
		"docker compose pull web\n"+
		"docker compose up -d --force-recreate web", svc.GenerateUpdateCommand(composeWeb("nginx:latest"), "latest", "sha256:new"))
}

func TestScanner_LocallyBuiltImage_IsSkipped(t *testing.T) {
	for _, image := range []string{"myapp:latest", "ghcr.io/acme/app:1.0.0"} {
		reg := &stubRegistry{
			tags: map[string][]string{
				"library/myapp":    {"latest"},
				"ghcr.io/acme/app": {"1.0.0", "2.0.0"},
			},
			digest: "sha256:new",
		}
		sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "latest")})

		results, errs := sc.Scan(context.Background(), []ContainerInfo{
			{ExternalID: "ctr1", Name: "app", Image: image, LocallyBuilt: true},
		})
		require.Empty(t, errs)
		assert.Empty(t, results, image)
		assert.Zero(t, reg.listCalls, "a locally built image has no registry to ask")
	}
}

func TestScanner_PreviousDigestIsTheRunningImage(t *testing.T) {
	reg := &stubRegistry{
		tags:   map[string][]string{"library/nginx": {"latest"}},
		digest: "sha256:new",
	}
	sc := newTestScanner(reg, &stubStore{baseline: oldBaseline("ctr1", "latest")})

	results, errs := sc.Scan(context.Background(), []ContainerInfo{{
		ExternalID: "ctr1", Name: "web", Image: "nginx:latest",
		RepoDigests: []string{"mirror.example.com/nginx@sha256:mirror", "docker.io/library/nginx@sha256:running"},
	}})
	require.Empty(t, errs)
	require.Len(t, results, 1)
	assert.Equal(t, "sha256:running", results[0].PreviousDigest)
}

// captureStore records the updates a scan persists.
type captureStore struct {
	*stubStore
	inserted []*ImageUpdate
}

func (s *captureStore) InsertImageUpdate(_ context.Context, u *ImageUpdate) (string, error) {
	s.inserted = append(s.inserted, u)
	return "", nil
}

func TestRunScan_PersistsThePreviousImageAndCarriesAlertOn(t *testing.T) {
	store := &captureStore{stubStore: &stubStore{}}
	reg := &stubRegistry{
		tags:   map[string][]string{"library/nginx": {"1.24.0", "1.26.0"}},
		digest: "sha256:latest",
	}

	var detected map[string]interface{}
	svc := NewService(Deps{
		Store:   store,
		Scanner: newTestScanner(reg, store),
		Containers: stubLister{containers: []ContainerInfo{{
			UID: "uid1", ExternalID: "ctr1", Name: "web", Image: "nginx:1.24.0",
			RepoDigests: []string{"nginx@sha256:running"},
			Labels:      map[string]string{"maintenant.update.alert_on": "critical"},
		}}},
		Logger: testLogger(),
		EventCallback: func(eventType string, data interface{}) {
			if eventType == event.UpdateDetected {
				detected, _ = data.(map[string]interface{})
			}
		},
	})

	svc.runScan(context.Background())

	require.Len(t, store.inserted, 1)
	assert.Equal(t, "sha256:running", store.inserted[0].PreviousDigest)
	require.NotNil(t, detected)
	assert.Equal(t, AlertOnCritical, detected["alert_on"])
	assert.Equal(t, "docker pull nginx@sha256:running\n"+
		"docker stop web && docker rm web\n"+
		"docker run -d --name web nginx@sha256:running", detected["rollback_command"])
}

func TestUpdateConfig_TrackLevel(t *testing.T) {
	cases := []struct {
		cfg  UpdateConfig
		want string
	}{
		{UpdateConfig{}, TrackMajor},
		{UpdateConfig{IgnoreMajor: true}, TrackMinor},
		{UpdateConfig{Track: TrackPatch, IgnoreMajor: true}, TrackPatch},
		{UpdateConfig{Track: TrackMinor, DigestOnly: true}, TrackDigest},
		{UpdateConfig{Track: TrackDigest}, TrackDigest},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.cfg.TrackLevel(), "%+v", tc.cfg)
	}
}
