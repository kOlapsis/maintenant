// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/container"
)

func assertUnbufferedEventStream(t *testing.T, h http.Header) {
	t.Helper()
	assert.Equal(t, "text/event-stream", h.Get("Content-Type"))
	assert.Equal(t, "no", h.Get("X-Accel-Buffering"), "nginx buffers the stream without it")
}

func TestSSEBroker_TellsProxiesNotToBuffer(t *testing.T) {
	broker := NewSSEBroker(slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	broker.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/containers/events", nil).WithContext(ctx))

	assertUnbufferedEventStream(t, rec.Header())
}

type chunkLogRequester struct {
	stubLogRequester
	lines []string
}

func (s *chunkLogRequester) SendCommand(context.Context, string, string, *agentpb.AgentCommand) (<-chan *agentpb.CommandResult, func(), error) {
	results := make(chan *agentpb.CommandResult, 1)
	results <- &agentpb.CommandResult{
		Last:   true,
		Result: &agentpb.CommandResult_Logs{Logs: &agentpb.LogsChunk{Lines: s.lines}},
	}
	close(results)
	return results, func() {}, nil
}

func TestHandleLogStream_RemoteTellsProxiesNotToBuffer(t *testing.T) {
	c := &container.Container{
		ID: "ctr-uuid", ExternalID: "deadbeef", Name: "web",
		AgentID: "11111111-2222-3333-4444-555555555555",
	}
	svc := container.NewService(container.Deps{
		Store:  &oneContainerStore{c: c},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	h := NewLogStreamHandler(nil, svc)
	h.SetLogRequester(&chunkLogRequester{lines: []string{"hello"}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/ctr-uuid/logs/stream", nil)
	req.SetPathValue("id", "ctr-uuid")
	rec := httptest.NewRecorder()
	h.HandleLogStream(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assertUnbufferedEventStream(t, rec.Header())
	assert.Contains(t, rec.Body.String(), "hello")
}
