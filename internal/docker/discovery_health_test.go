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

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

type healthFakeAPI struct {
	*fakeAPI
	state container.State
}

func (f *healthFakeAPI) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	state := f.state
	return client.ContainerInspectResult{Container: container.InspectResponse{
		Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		State:  &state,
	}}, nil
}

func TestDiscoverAll_ReadsHealthOnlyWhileTheCheckRuns(t *testing.T) {
	unhealthy := cmodel.HealthUnhealthy
	health := &container.Health{Status: container.Unhealthy}
	cases := []struct {
		name  string
		state container.State
		want  *cmodel.HealthStatus
	}{
		{"running", container.State{Status: "running", Running: true, Health: health}, &unhealthy},
		{"exited", container.State{Status: "exited", Health: health}, nil},
		{"restarting", container.State{Status: "restarting", Running: true, Restarting: true, Health: health}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &healthFakeAPI{
				fakeAPI: &fakeAPI{list: []container.Summary{{ID: "0123456789abcdef", Names: []string{"/job"}, State: tc.state.Status}}},
				state:   tc.state,
			}
			c := newFakeClient(api.fakeAPI)
			c.cli = api

			containers, err := c.DiscoverAll(context.Background())
			require.NoError(t, err)
			require.Len(t, containers, 1)
			assert.Equal(t, tc.want, containers[0].HealthStatus)
			assert.True(t, containers[0].HasHealthCheck)
		})
	}
}
