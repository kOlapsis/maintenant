// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var floatingTags = map[string][]string{
	"library/nginx":    {"latest", "1.27.0"},
	"library/traefik":  {"v3", "v3.7.10"},
	"ghcr.io/acme/api": {"stable", "1.0.0"},
}

// The update is reported on every scan while the container runs the old digest, and only then resolves.
func TestScanner_DigestUpdateLastsUntilTheContainerRunsTheNewDigest(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:new"}
	store := &stubStore{baseline: oldBaseline("ctr1", "latest")}
	sc := newTestScanner(reg, store)
	running := ContainerInfo{
		ExternalID: "ctr1", Name: "web", Image: "nginx:latest",
		RepoDigests: []string{"nginx@sha256:old"},
	}

	for scan := 1; scan <= 3; scan++ {
		results, errs := sc.Scan(context.Background(), []ContainerInfo{running})
		require.Empty(t, errs)
		require.Len(t, results, 1, "scan %d", scan)
		assert.Equal(t, "sha256:old", results[0].CurrentDigest)
		assert.Equal(t, "sha256:new", results[0].LatestDigest)
	}

	running.RepoDigests = []string{"nginx@sha256:new"}
	results, errs := sc.Scan(context.Background(), []ContainerInfo{running})
	require.Empty(t, errs)
	assert.Empty(t, results)
}

// A Kubernetes workload keeps its id across a rollout, so the reference is the digest its pods run.
func TestScanner_WorkloadWithUnchangedIDResolvesOnceItRunsTheNewDigest(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:new"}
	sc := newTestScanner(reg, &stubStore{})
	workload := ContainerInfo{
		ExternalID: "prod/Deployment/api", Name: "api", Image: "ghcr.io/acme/api:stable",
		RuntimeType: "kubernetes", ControllerKind: "Deployment",
		RepoDigests: []string{"ghcr.io/acme/api@sha256:old"},
	}

	results, _ := sc.Scan(context.Background(), []ContainerInfo{workload})
	require.Len(t, results, 1)

	workload.RepoDigests = []string{"ghcr.io/acme/api@sha256:new"}
	results, _ = sc.Scan(context.Background(), []ContainerInfo{workload})
	assert.Empty(t, results)
}

// Runtimes record the digest of the multi-platform index they pulled, or of the one platform manifest.
func TestScanner_RunningDigestMatchesTheIndexOrOneOfItsPlatforms(t *testing.T) {
	for _, running := range []string{"sha256:index", "sha256:arm64"} {
		reg := &stubRegistry{tags: floatingTags, digest: "sha256:index", platforms: []string{"sha256:amd64", "sha256:arm64"}}
		store := &stubStore{baseline: oldBaseline("ctr1", "latest")}
		sc := newTestScanner(reg, store)

		results, errs := sc.Scan(context.Background(), []ContainerInfo{{
			ExternalID: "ctr1", Name: "web", Image: "nginx:latest",
			RepoDigests: []string{"docker.io/library/nginx@" + running},
		}})
		require.Empty(t, errs)
		assert.Empty(t, results, running)
		assert.Equal(t, "sha256:index", store.baseline.RemoteDigest, "the baseline keeps the digest valid on every platform")
	}
}

// A baseline naming one platform manifest, as the server's own platform was once recorded, gives way to the multi-platform digest.
func TestScanner_UnreportedDigest_PlatformBaselineBecomesTheIndexDigest(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:index1", platforms: []string{"sha256:amd64-1", "sha256:arm64-1"}}
	store := &stubStore{baseline: &DigestBaseline{ContainerID: "ctr1", Tag: "latest", RemoteDigest: "sha256:amd64-1"}}
	sc := newTestScanner(reg, store)
	armHost := ContainerInfo{ExternalID: "ctr1", Name: "web", Image: "nginx:latest"}

	results, _ := sc.Scan(context.Background(), []ContainerInfo{armHost})
	assert.Empty(t, results)
	assert.Equal(t, "sha256:index1", store.baseline.RemoteDigest)

	reg.digest, reg.platforms = "sha256:index2", []string{"sha256:amd64-2", "sha256:arm64-2"}
	results, _ = sc.Scan(context.Background(), []ContainerInfo{armHost})
	require.Len(t, results, 1)
	assert.Equal(t, "sha256:index1", results[0].PreviousDigest, "a rollback must not pin the server's platform")
}

// A container whose runtime does not report its digest is compared with what its tag pointed at on first sight, a multi-platform digest.
func TestScanner_UnreportedDigest_BaselineIsTheIndexDigest(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:index1", platforms: []string{"sha256:amd64-1", "sha256:arm64-1"}}
	store := &stubStore{}
	sc := newTestScanner(reg, store)
	remote := ContainerInfo{ExternalID: "ctr1", Name: "web", Image: "nginx:latest"}

	results, _ := sc.Scan(context.Background(), []ContainerInfo{remote})
	assert.Empty(t, results, "first sight only records the reference")
	require.NotNil(t, store.baseline)
	assert.Equal(t, "sha256:index1", store.baseline.RemoteDigest)

	reg.digest, reg.platforms = "sha256:index2", []string{"sha256:amd64-2", "sha256:arm64-2"}
	for range 2 {
		results, _ = sc.Scan(context.Background(), []ContainerInfo{remote})
		require.Len(t, results, 1)
		assert.Equal(t, "sha256:index1", results[0].PreviousDigest)
	}

	svc := &Service{}
	u := &ImageUpdate{Image: "nginx:latest", CurrentTag: "latest", LatestTag: "latest", PreviousDigest: results[0].PreviousDigest}
	assert.Contains(t, svc.GenerateRollbackCommand(remote, u), "nginx@sha256:index1")
}

// A baseline recorded for another tag says nothing about the current one.
func TestScanner_UnreportedDigest_BaselineOfAnotherTagIsReplaced(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:new"}
	store := &stubStore{baseline: oldBaseline("ctr1", "v2")}
	sc := newTestScanner(reg, store)

	results, _ := sc.Scan(context.Background(), []ContainerInfo{{ExternalID: "ctr1", Name: "proxy", Image: "traefik:v3"}})
	assert.Empty(t, results)
	assert.Equal(t, "v3", store.baseline.Tag)
	assert.Equal(t, "sha256:new", store.baseline.RemoteDigest)
}

// The last digest seen running stands in while the runtime cannot tell it, as during a rollout.
func TestScanner_RunningDigestIsRecordedForWhenItIsUnknown(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:new"}
	store := &stubStore{}
	sc := newTestScanner(reg, store)
	workload := ContainerInfo{
		ExternalID: "prod/Deployment/api", Name: "api", Image: "ghcr.io/acme/api:stable",
		RepoDigests: []string{"ghcr.io/acme/api@sha256:old"},
	}

	results, _ := sc.Scan(context.Background(), []ContainerInfo{workload})
	require.Len(t, results, 1)

	workload.RepoDigests = nil
	results, _ = sc.Scan(context.Background(), []ContainerInfo{workload})
	require.Len(t, results, 1)
	assert.Equal(t, "sha256:old", results[0].CurrentDigest)
}

func TestScanner_DigestPinnedInTheReferenceIsTheRunningOne(t *testing.T) {
	reg := &stubRegistry{tags: floatingTags, digest: "sha256:new"}
	sc := newTestScanner(reg, &stubStore{})

	results, _ := sc.Scan(context.Background(), []ContainerInfo{{
		ExternalID: "task1", Name: "web.1", Image: "nginx:latest@sha256:old",
	}})
	require.Len(t, results, 1)
	assert.Equal(t, "sha256:old", results[0].CurrentDigest)
}
