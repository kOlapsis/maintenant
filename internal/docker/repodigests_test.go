// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type imageFakeAPI struct {
	*fakeAPI
	images []image.Summary
}

func (f *imageFakeAPI) ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{Items: f.images}, nil
}

func TestContainerRepoDigests(t *testing.T) {
	api := &imageFakeAPI{
		fakeAPI: &fakeAPI{list: []container.Summary{
			{ID: "pulled", ImageID: "sha256:img-pulled"},
			{ID: "built", ImageID: "sha256:img-built"},
			{ID: "gone", ImageID: "sha256:img-gone"},
		}},
		images: []image.Summary{
			{ID: "sha256:img-pulled", RepoDigests: []string{"nginx@sha256:abc"}},
			{ID: "sha256:img-built", RepoTags: []string{"myapp:latest"}},
		},
	}
	c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	got, err := c.ContainerRepoDigests(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"nginx@sha256:abc"}, got["pulled"])
	built, ok := got["built"]
	assert.True(t, ok)
	assert.Empty(t, built)
	_, ok = got["gone"]
	assert.False(t, ok, "a container whose image is not listed stays unknown")
}
