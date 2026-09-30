// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/container"
	"github.com/kolapsis/maintenant/internal/docker"
)

type imageListRefused struct{}

func (imageListRefused) DiscoverAllWithLabels(context.Context) ([]*docker.DiscoveryResult, error) {
	return []*docker.DiscoveryResult{{
		Container: &container.Container{ExternalID: "c1", Name: "web", Image: "nginx:latest"},
		Labels:    map[string]string{"maintenant.update.track": "digest"},
	}}, nil
}

func (imageListRefused) ContainerRepoDigests(context.Context) (map[string][]string, error) {
	return nil, errors.New("image list: Error response from daemon: 403 Forbidden")
}

// Behind docker-socket-proxy without IMAGES=1 the scan keeps every container, and says why digests are missing once.
func TestDockerDetailsFetcher_ImageListRefused(t *testing.T) {
	logs := &syncBuffer{}
	f := &dockerDetailsFetcher{rt: imageListRefused{}, logger: slog.New(slog.NewTextHandler(logs, nil))}

	for range 3 {
		details, err := f.FetchDetails(context.Background())
		require.NoError(t, err)
		require.Contains(t, details, "c1", "the container stays in the scan")
		assert.Equal(t, "digest", details["c1"].Labels["maintenant.update.track"])
		assert.False(t, details["c1"].DigestsKnown)
	}

	assert.Equal(t, 1, strings.Count(logs.String(), "IMAGES=1"), "the refusal is logged once, not at every scan")
	assert.Contains(t, logs.String(), "level=WARN")
}
