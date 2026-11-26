// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statsFakeAPI struct {
	fakeAPI
}

func (f *statsFakeAPI) ContainerStats(context.Context, string, client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	return client.ContainerStatsResult{Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}

func TestRuntime_ForgetsCPUStateOfContainersThatAreGone(t *testing.T) {
	const kept, gone = "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"
	api := &statsFakeAPI{fakeAPI{list: []container.Summary{
		{ID: kept, Names: []string{"/kept"}, State: "running"},
		{ID: gone, Names: []string{"/gone"}, State: "running"},
	}}}
	rt := &Runtime{client: newFakeClient(&api.fakeAPI), prev: make(map[string]*cpuPrev)}
	rt.client.cli = api
	ctx := context.Background()

	for _, id := range []string{kept, gone} {
		_, err := rt.StatsSnapshot(ctx, id)
		require.NoError(t, err)
	}
	require.Len(t, rt.prev, 2)

	api.list = api.list[:1]
	_, err := rt.DiscoverAll(ctx)
	require.NoError(t, err)

	assert.Len(t, rt.prev, 1)
	assert.Contains(t, rt.prev, kept)
}
