// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	client.APIClient
	list   []container.Summary
	events chan events.Message
}

func (f *fakeAPI) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: f.list}, nil
}

func (f *fakeAPI) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, errors.New("inspect unavailable")
}

func (f *fakeAPI) Events(context.Context, client.EventsListOptions) client.EventsResult {
	return client.EventsResult{Messages: f.events, Err: make(chan error)}
}

const caddyLabel = "caddy"

func newFakeClient(api *fakeAPI) *Client {
	return &Client{cli: api, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestDiscoverAllWithLabels_ProxyLabelsOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		api := &fakeAPI{list: []container.Summary{{
			ID:     "0123456789abcdef",
			Names:  []string{"/web"},
			State:  "running",
			Labels: map[string]string{caddyLabel: "app.example.com"},
		}}}
		c := newFakeClient(api)
		c.SetProxyLabels(enabled)

		results, err := c.DiscoverAllWithLabels(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 1)

		got, ok := results[0].Labels["maintenant.endpoint.0.http"]
		assert.Equal(t, enabled, ok, "enabled=%v", enabled)
		if enabled {
			assert.Equal(t, "https://app.example.com", got)
		}
		_, leaked := api.list[0].Labels["maintenant.endpoint.0.http"]
		assert.False(t, leaked, "the Docker summary labels must not be mutated")
	}
}

func TestStreamEvents_ProxyLabelsOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		api := &fakeAPI{events: make(chan events.Message, 1)}
		api.events <- events.Message{
			Type:   events.ContainerEventType,
			Action: "start",
			Actor: events.Actor{
				ID:         "ctr1",
				Attributes: map[string]string{"name": "web", caddyLabel: "app.example.com"},
			},
			Time: 1,
		}
		c := newFakeClient(api)
		c.SetProxyLabels(enabled)

		ctx, cancel := context.WithCancel(context.Background())
		ch := c.StreamEvents(ctx)
		select {
		case evt := <-ch:
			_, ok := evt.Labels["maintenant.endpoint.0.http"]
			assert.Equal(t, enabled, ok, "enabled=%v", enabled)
		case <-time.After(2 * time.Second):
			t.Fatal("no event received")
		}
		cancel()
	}
}
