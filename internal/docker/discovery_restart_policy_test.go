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

type restartPolicyFakeAPI struct {
	*fakeAPI
	policy container.RestartPolicyMode
}

func (f *restartPolicyFakeAPI) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: container.InspectResponse{
		HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: f.policy}},
		State:      &container.State{Status: "running", Running: true},
	}}, nil
}

func TestDiscoverAll_ReadsWhetherTheRestartPolicyRestartsACleanExit(t *testing.T) {
	for policy, want := range map[container.RestartPolicyMode]bool{
		container.RestartPolicyAlways:        true,
		container.RestartPolicyUnlessStopped: true,
		container.RestartPolicyOnFailure:     false,
		container.RestartPolicyDisabled:      false,
		"":                                   false,
	} {
		t.Run(string(policy), func(t *testing.T) {
			api := &restartPolicyFakeAPI{
				fakeAPI: &fakeAPI{list: []container.Summary{{ID: "0123456789abcdef", Names: []string{"/web"}, State: "running"}}},
				policy:  policy,
			}
			c := newFakeClient(api.fakeAPI)
			c.cli = api

			containers, err := c.DiscoverAll(context.Background())
			require.NoError(t, err)
			require.Len(t, containers, 1)
			assert.Equal(t, want, containers[0].RestartsOnCleanExit)
		})
	}
}
