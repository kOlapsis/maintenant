// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
)

func shortKeepAlive(t *testing.T) {
	t.Helper()
	prev := sseKeepAliveInterval
	sseKeepAliveInterval = 10 * time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval = prev })
}

// requireKeepAlive opens the stream and waits for a keep-alive comment on an otherwise silent stream.
func requireKeepAlive(t *testing.T, h http.Handler, path string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadString('\n')
		require.NoError(t, err, "the stream ended or stalled without a keep-alive")
		if strings.TrimRight(line, "\n") == ": keepalive" {
			return
		}
	}
}

func TestSSEBroker_KeepsAnIdleStreamAlive(t *testing.T) {
	shortKeepAlive(t)
	broker := NewSSEBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	requireKeepAlive(t, broker, "/api/v1/containers/events")
}

type silentLogStreamer struct {
	reader *io.PipeReader
}

func (s silentLogStreamer) StreamLogs(context.Context, string, int, bool) (io.ReadCloser, error) {
	return s.reader, nil
}

func TestHandleLogStream_KeepsAQuietLocalContainerAlive(t *testing.T) {
	shortKeepAlive(t)
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	h := NewLogStreamHandler(silentLogStreamer{reader: pr}, nil)

	requireKeepAlive(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "deadbeef")
		h.HandleLogStream(w, r)
	}), "/api/v1/containers/deadbeef/logs/stream")
}

type silentLogRequester struct {
	stubLogRequester
	results chan *agentpb.CommandResult
}

func (s *silentLogRequester) SendCommand(context.Context, string, string, *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error) {
	return s.results, func() {}, nil
}

func TestHandleLogStream_KeepsAQuietRemoteContainerAlive(t *testing.T) {
	shortKeepAlive(t)
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "deadbeef", Name: "web",
		AgentID: "11111111-2222-3333-4444-555555555555",
	}
	svc := container.NewService(container.Deps{
		Store:  &oneContainerStore{c: c},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	h := NewLogStreamHandler(nil, svc)
	h.SetLogRequester(&silentLogRequester{results: make(chan *agentpb.CommandResult)})

	requireKeepAlive(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("id", "ctr-uuid")
		h.HandleLogStream(w, r)
	}), "/api/v1/containers/ctr-uuid/logs/stream")
}
