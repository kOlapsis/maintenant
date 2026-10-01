// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type movedTagAPI struct {
	*fakeAPI
	configImage string
}

func (f *movedTagAPI) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: container.InspectResponse{
		Config: &container.Config{Image: f.configImage},
		State:  &container.State{Status: "running"},
	}}, nil
}

// After `docker pull` moves the tag, the list reports the running image by its ID.
func TestDiscoverAll_KeepsTheReferenceOfAMovedTag(t *testing.T) {
	api := &movedTagAPI{
		fakeAPI: &fakeAPI{list: []container.Summary{{
			ID:      "0123456789abcdef",
			Names:   []string{"/web"},
			Image:   "sha256:5f1d7a9e3c2b",
			ImageID: "sha256:5f1d7a9e3c2b",
			State:   "running",
		}}},
		configImage: "nginx:1.27",
	}
	c := newFakeClient(api.fakeAPI)
	c.cli = api

	containers, err := c.DiscoverAll(context.Background())
	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.Equal(t, "nginx:1.27", containers[0].Image)
}
