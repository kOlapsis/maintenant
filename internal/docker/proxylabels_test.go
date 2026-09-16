// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package docker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	client.APIClient
	list   []container.Summary
	events chan events.Message
}

func (f *fakeAPI) ContainerList(context.Context, container.ListOptions) ([]container.Summary, error) {
	return f.list, nil
}

func (f *fakeAPI) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, errors.New("inspect unavailable")
}

func (f *fakeAPI) Events(context.Context, events.ListOptions) (<-chan events.Message, <-chan error) {
	return f.events, make(chan error)
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
