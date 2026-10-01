// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processEvent must surface the container image from the Docker event actor
// attributes so agents can forward it to the server.
func TestProcessEvent_ExtractsImage(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "start",
		Actor: events.Actor{
			ID:         "ctr123",
			Attributes: map[string]string{"name": "demo", "image": "adminer:latest"},
		},
		Time: 1,
	}

	evt := processEvent(msg)
	require.NotNil(t, evt)
	assert.Equal(t, "container", evt.ResourceType)
	assert.Equal(t, "ctr123", evt.ExternalID)
	assert.Equal(t, "demo", evt.Name)
	assert.Equal(t, "adminer:latest", evt.Image)
}

func TestProcessEvent_DieCarriesImageAndExitCode(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "die",
		Actor: events.Actor{
			ID:         "ctr456",
			Attributes: map[string]string{"name": "crash", "image": "app:v1", "exitCode": "137"},
		},
		Time: 1,
	}

	evt := processEvent(msg)
	require.NotNil(t, evt)
	assert.Equal(t, "app:v1", evt.Image)
	assert.Equal(t, "137", evt.ExitCode)
}

func TestProcessEvent_IgnoresUnknownAction(t *testing.T) {
	msg := events.Message{
		Type:   events.ContainerEventType,
		Action: "exec_create",
		Actor:  events.Actor{ID: "x", Attributes: map[string]string{}},
		Time:   1,
	}
	assert.Nil(t, processEvent(msg))
}

type subscription struct {
	msgs []events.Message
	end  error
}

// scriptedEventsAPI serves one scripted subscription per Events call, then keeps the stream open.
type scriptedEventsAPI struct {
	client.APIClient
	mu      sync.Mutex
	subs    []subscription
	since   []string
	pingErr error
}

func (f *scriptedEventsAPI) Events(ctx context.Context, opts client.EventsListOptions) client.EventsResult {
	f.mu.Lock()
	f.since = append(f.since, opts.Since)
	var sub subscription
	if len(f.subs) > 0 {
		sub, f.subs = f.subs[0], f.subs[1:]
	}
	f.mu.Unlock()

	msgs := make(chan events.Message)
	errs := make(chan error, 1)
	go func() {
		for _, m := range sub.msgs {
			select {
			case msgs <- m:
			case <-ctx.Done():
				return
			}
		}
		if sub.end != nil {
			errs <- sub.end
		}
	}()
	return client.EventsResult{Messages: msgs, Err: errs}
}

func (f *scriptedEventsAPI) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return client.PingResult{}, f.pingErr
}

func (f *scriptedEventsAPI) subscribedSince() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.since...)
}

func started(id string, at time.Time) events.Message {
	return events.Message{
		Type:     events.ContainerEventType,
		Action:   "start",
		Actor:    events.Actor{ID: id, Attributes: map[string]string{"name": id}},
		Time:     at.Unix(),
		TimeNano: at.UnixNano(),
	}
}

func nextEvent(t *testing.T, ch <-chan ContainerEvent) ContainerEvent {
	t.Helper()
	select {
	case evt, ok := <-ch:
		require.True(t, ok, "the stream closed")
		return evt
	case <-time.After(5 * time.Second):
		t.Fatal("no event received")
		return ContainerEvent{}
	}
}

func TestStreamEvents_ResumesACutStreamAfterTheLastEventWhileTheDaemonAnswers(t *testing.T) {
	at := time.Date(2026, 9, 30, 10, 0, 0, 123456789, time.UTC)
	api := &scriptedEventsAPI{subs: []subscription{
		{msgs: []events.Message{started("ctr1", at)}, end: io.ErrUnexpectedEOF},
		{msgs: []events.Message{started("ctr2", at.Add(time.Second))}},
	}}
	c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), connected: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.StreamEvents(ctx)

	assert.Equal(t, "ctr1", nextEvent(t, ch).ExternalID)
	assert.Equal(t, "ctr2", nextEvent(t, ch).ExternalID, "a cut stream is resumed while the daemon answers")
	assert.True(t, c.IsConnected())

	since := api.subscribedSince()
	require.Len(t, since, 2)
	resumed, err := time.Parse(time.RFC3339Nano, since[1])
	require.NoError(t, err)
	assert.Equal(t, at.Add(time.Nanosecond), resumed.UTC(), "the new subscription starts right after the last event received")
}

func TestStreamEvents_ClosesWhenTheDaemonIsLost(t *testing.T) {
	api := &scriptedEventsAPI{
		subs:    []subscription{{msgs: []events.Message{started("ctr1", time.Now())}, end: io.EOF}},
		pingErr: errors.New("dial unix /var/run/docker.sock: connect: no such file or directory"),
	}
	var logs bytes.Buffer
	c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(&logs, nil)), connected: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := c.StreamEvents(ctx)

	assert.Equal(t, "ctr1", nextEvent(t, ch).ExternalID)
	select {
	case _, ok := <-ch:
		require.False(t, ok, "no event follows the loss")
	case <-time.After(5 * time.Second):
		t.Fatal("the stream stayed open although the daemon no longer answers")
	}
	assert.False(t, c.IsConnected())
	assert.Contains(t, logs.String(), "Docker daemon lost")
	assert.Contains(t, logs.String(), "no such file or directory", "the cause is logged")
}

func TestStreamEvents_EndsQuietlyWithItsContext(t *testing.T) {
	api := &scriptedEventsAPI{pingErr: errors.New("unreachable")}
	var logs bytes.Buffer
	c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(&logs, nil)), connected: true}

	ctx, cancel := context.WithCancel(context.Background())
	ch := c.StreamEvents(ctx)
	cancel()
	select {
	case _, ok := <-ch:
		require.False(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not end with its context")
	}
	assert.True(t, c.IsConnected(), "a shutdown is not a daemon loss")
	assert.NotContains(t, logs.String(), "Docker daemon lost")
}
