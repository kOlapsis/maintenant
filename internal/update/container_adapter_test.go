// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package update

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
)

type fakeContainerStore struct {
	container.ContainerStore
	list []*container.Container
}

func (f fakeContainerStore) ListContainers(context.Context, container.ListContainersOpts) ([]*container.Container, error) {
	return f.list, nil
}

type fakeRepoDigests map[string][]string

func (f fakeRepoDigests) FetchRepoDigests(context.Context) (map[string][]string, error) {
	return f, nil
}

func TestContainerServiceAdapter_RepoDigestsAndLocalBuilds(t *testing.T) {
	svc := container.NewService(container.Deps{
		Store: fakeContainerStore{list: []*container.Container{
			{ExternalID: "pulled", Name: "web", Image: "nginx:latest"},
			{ExternalID: "built", Name: "app", Image: "myapp:latest"},
			{ExternalID: "unknown", Name: "remote", Image: "redis:latest"},
		}},
		Logger: testLogger(),
	})
	adapter := NewContainerServiceAdapter(svc).WithRepoDigestFetcher(fakeRepoDigests{
		"pulled": {"nginx@sha256:abc"},
		"built":  {},
	})

	infos, err := adapter.ListContainerInfos(context.Background())
	require.NoError(t, err)
	require.Len(t, infos, 3)

	byID := map[string]ContainerInfo{}
	for _, info := range infos {
		byID[info.ExternalID] = info
	}
	assert.Equal(t, []string{"nginx@sha256:abc"}, byID["pulled"].RepoDigests)
	assert.False(t, byID["pulled"].LocallyBuilt)
	assert.True(t, byID["built"].LocallyBuilt)
	assert.False(t, byID["unknown"].LocallyBuilt, "a runtime that says nothing about the image must not hide it")
}
