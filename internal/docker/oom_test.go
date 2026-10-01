// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmodel "github.com/kolapsis/maintenant/internal/container"
)

type exitedFakeAPI struct {
	*fakeAPI
	exitCode  int
	oomKilled bool
}

func (f *exitedFakeAPI) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: container.InspectResponse{
		ID: id,
		State: &container.State{
			Status:    "exited",
			ExitCode:  f.exitCode,
			OOMKilled: f.oomKilled,
		},
	}}, nil
}

func TestDiscoverAllWithLabels_OOMKillIsACrash(t *testing.T) {
	cases := []struct {
		name      string
		exitCode  int
		oomKilled bool
		want      cmodel.ContainerState
	}{
		{"docker stop after the grace period", 137, false, cmodel.StateCompleted},
		{"SIGTERM", 143, false, cmodel.StateCompleted},
		{"clean exit", 0, false, cmodel.StateCompleted},
		{"OOM kill", 137, true, cmodel.StateExited},
		{"crash", 1, false, cmodel.StateExited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &exitedFakeAPI{
				fakeAPI:   &fakeAPI{list: []container.Summary{{ID: "0123456789abcdef", Names: []string{"/job"}, State: "exited"}}},
				exitCode:  tc.exitCode,
				oomKilled: tc.oomKilled,
			}
			c := &Client{cli: api, logger: newFakeClient(api.fakeAPI).logger}

			results, err := c.DiscoverAllWithLabels(context.Background())
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tc.want, results[0].Container.State)
			require.NotNil(t, results[0].Exit)
			assert.Equal(t, ExitInfo{Code: tc.exitCode, OOMKilled: tc.oomKilled}, *results[0].Exit)
		})
	}
}

func TestStreamEvents_Die137CarriesTheOOMFlag(t *testing.T) {
	for _, oom := range []bool{false, true} {
		api := &exitedFakeAPI{fakeAPI: &fakeAPI{events: make(chan events.Message, 1)}, exitCode: 137, oomKilled: oom}
		api.events <- events.Message{
			Type:   events.ContainerEventType,
			Action: "die",
			Actor:  events.Actor{ID: "ctr1", Attributes: map[string]string{"name": "web", "exitCode": "137"}},
			Time:   1,
		}
		r := &Runtime{client: &Client{cli: api, logger: newFakeClient(api.fakeAPI).logger}}

		ctx, cancel := context.WithCancel(context.Background())
		select {
		case evt := <-r.StreamEvents(ctx):
			assert.Equal(t, "137", evt.ExitCode)
			assert.Equal(t, oom, evt.OOMKilled)
		case <-time.After(2 * time.Second):
			t.Fatal("no event received")
		}
		cancel()
	}
}
